package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestLargeContentReadUsesDiskFallback(t *testing.T) {
	ctx := context.Background()
	metadata := newMemoryMeta()
	objects := &countedObjects{Mem: objecttest.NewMem()}
	m, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "large", AccountID: "acct", StorageVersion: 2, DefaultBranch: "main"}
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if err := m.ensure(ctx, repo, path); err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("large file\n", 900000)
	sha, err := writeCommit(ctx, path, t.TempDir(), "", types.CommitInput{Files: []types.CommitFile{{Path: "large", Content: content}}})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := m.packAndUpload(ctx, repo, path)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := metadata.WAL().Publish(ctx, types.Publication{Pack: pack, Updates: []types.RefUpdate{{Name: "refs/heads/main", NewSHA: sha}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = seq
	if err := m.Evict(repo); err != nil {
		t.Fatal(err)
	}
	readRemoteFile(t, m, repo, sha, "large", content)
	if objects.fullPacks == 0 {
		t.Fatal("large read did not use disk cache")
	}
	reader := m.diskObjectReader(ctx, repo, path)
	for range 2 {
		if _, err := reader(plumbing.CommitObject, plumbing.NewHash(sha)); err != nil {
			t.Fatal(err)
		}
	}

	fallback, err := New(metadata, &noRanges{objects.Mem}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := fallback.ReadContent(ctx, repo, func(storer.Storer) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type noRanges struct{ object.Store }
