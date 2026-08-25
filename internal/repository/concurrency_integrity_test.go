package repository

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestCrossManagerLockAndScopedRead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	metadata, objects, root := newMemoryMeta(), objecttest.NewMem(), t.TempDir()
	one, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	two, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "locked", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	result, err := one.Commit(context.Background(), repo, types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = result.Sequence
	done := make(chan error, 1)
	err = one.Read(context.Background(), repo, func(store storer.Storer) error {
		go func() { done <- two.Evict(repo) }()
		select {
		case err := <-done:
			return errors.New("cross-manager operation escaped lock: " + err.Error())
		case <-time.After(30 * time.Millisecond):
		}
		_, err := store.Reference("refs/heads/main")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentCacheOperationsAndCorruptReplay(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	metadata, objects := newMemoryMeta(), objecttest.NewMem()
	manager, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "concurrent", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	result, err := manager.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = result.Sequence
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for _, operation := range []func() error{
		func() error { return manager.Read(ctx, repo, func(storer.Storer) error { return nil }) },
		func() error { return manager.Compact(ctx, repo) },
		func() error { return manager.Evict(repo) },
	} {
		wg.Add(1)
		go func(run func() error) { defer wg.Done(); errs <- run() }(operation)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent cache operation: %v", err)
		}
	}
	assertReplayIntegrity(t, ctx, manager, metadata, objects, repo)
}

func assertReplayIntegrity(t *testing.T, ctx context.Context, manager *Manager, metadata *memoryMeta, objects *objecttest.Mem, repo types.Repo) {
	t.Helper()
	pack := metadata.packs[repo.ID][0]
	index, err := objects.Get(ctx, pack.IndexKey)
	if err != nil {
		t.Fatal(err)
	}
	indexBytes, err := io.ReadAll(index)
	_ = index.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.Delete(ctx, pack.IndexKey); err != nil {
		t.Fatal(err)
	}
	if err := manager.Evict(repo); err != nil {
		t.Fatal(err)
	}
	if err := manager.Read(ctx, repo, func(storer.Storer) error { return nil }); err == nil {
		t.Fatal("rebuilt cache from partial immutable pair")
	}
	if err := objects.Put(ctx, pack.IndexKey, bytes.NewReader(indexBytes), int64(len(indexBytes))); err != nil {
		t.Fatal(err)
	}
	if err := manager.Read(ctx, repo, func(storer.Storer) error { return nil }); err != nil {
		t.Fatalf("partial pair retry: %v", err)
	}
	if err := objects.Delete(ctx, pack.PackKey); err != nil {
		t.Fatal(err)
	}
	if err := objects.Put(ctx, pack.PackKey, bytes.NewBufferString("corrupt"), 7); err != nil {
		t.Fatal(err)
	}
	if err := manager.Evict(repo); err != nil {
		t.Fatal(err)
	}
	err = manager.Read(ctx, repo, func(storer.Storer) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupt immutable replay %v", err)
	}
	if err := manager.Evict(repo); err != nil {
		t.Fatalf("corruption eviction failed: %v", err)
	}
}
