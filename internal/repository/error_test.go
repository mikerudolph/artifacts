package repository

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

type failingMeta struct {
	*memoryMeta
	refsErr       error
	walErr        error
	checkpointErr error
	forkErr       error
}

func (m *failingMeta) Refs() meta.Refs { return failingRefs{Refs: m.memoryMeta.Refs(), err: m.refsErr} }
func (m *failingMeta) WAL() meta.WAL   { return failingWAL{WAL: m.memoryMeta.WAL(), err: m.walErr} }
func (m *failingMeta) Checkpoints() meta.Checkpoints {
	return failingCheckpoints{Checkpoints: m.memoryMeta.Checkpoints(), err: m.checkpointErr}
}
func (m *failingMeta) Forks() meta.Forks {
	return failingForks{Forks: m.memoryMeta.Forks(), err: m.forkErr}
}

type failingRefs struct {
	meta.Refs
	err error
}

func (s failingRefs) List(ctx context.Context, repo types.RepoID) ([]types.Ref, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.Refs.List(ctx, repo)
}

type failingWAL struct {
	meta.WAL
	err error
}

func (s failingWAL) List(ctx context.Context, repo types.RepoID, after, through int64) ([]types.PackWAL, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.WAL.List(ctx, repo, after, through)
}

type failingCheckpoints struct {
	meta.Checkpoints
	err error
}

func (s failingCheckpoints) Get(ctx context.Context, repo types.RepoID) (types.Checkpoint, error) {
	if s.err != nil {
		return types.Checkpoint{}, s.err
	}
	return s.Checkpoints.Get(ctx, repo)
}

type failingForks struct {
	meta.Forks
	err error
}

func (s failingForks) Get(ctx context.Context, repo types.RepoID) (types.ForkLineage, error) {
	if s.err != nil {
		return types.ForkLineage{}, s.err
	}
	return s.Forks.Get(ctx, repo)
}

func TestMetadataFailuresAbortCacheRebuild(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	boom := errors.New("metadata unavailable")
	tests := []struct {
		name string
		set  func(*failingMeta)
	}{
		{name: "fork lineage", set: func(m *failingMeta) { m.forkErr = boom }},
		{name: "checkpoint", set: func(m *failingMeta) { m.checkpointErr = boom }},
		{name: "wal", set: func(m *failingMeta) { m.walErr = boom }},
		{name: "refs", set: func(m *failingMeta) { m.refsErr = boom }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := &failingMeta{memoryMeta: newMemoryMeta()}
			test.set(metadata)
			manager, err := New(metadata, objecttest.NewMem(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			repo := types.Repo{ID: "repo", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
			if err := manager.Read(context.Background(), repo, func(storer.Storer) error { return nil }); !errors.Is(err, boom) {
				t.Fatalf("open error %v", err)
			}
		})
	}
}

func TestRepositoryEntryErrors(t *testing.T) {
	manager, err := New(newMemoryMeta(), objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := manager.lockedPath(types.Repo{}); err == nil {
		t.Fatal("accepted empty repo")
	}
	if err := manager.Read(ctx, types.Repo{}, func(storer.Storer) error { return nil }); err == nil {
		t.Fatal("opened empty repo")
	}
	if err := manager.RPC(ctx, types.Repo{}, "upload-pack", nil, io.Discard, ""); err == nil {
		t.Fatal("rpc opened empty repo")
	}
	if _, err := manager.Receive(ctx, types.Repo{}, strings.NewReader("bad"), ""); err == nil {
		t.Fatal("receive opened empty repo")
	}
	if err := manager.Compact(ctx, types.Repo{}); err == nil {
		t.Fatal("compacted empty repo")
	}
	if err := manager.Import(ctx, types.Repo{}, "file:///bad", "", 0); err == nil {
		t.Fatal("imported empty repo")
	}
	if _, err := manager.Commit(ctx, types.Repo{DefaultBranch: "main"}, types.CommitInput{}); err == nil {
		t.Fatal("committed invalid input")
	}
	valid := types.CommitInput{Files: []types.CommitFile{{Path: "README.md", Content: "x"}}}
	if _, err := manager.Commit(ctx, types.Repo{DefaultBranch: "main"}, valid); err == nil {
		t.Fatal("committed without repository identity")
	}
	if err := manager.Evict(types.Repo{}); err == nil {
		t.Fatal("evicted empty repo")
	}
}

func TestRepositoryStorageErrors(t *testing.T) {
	metadata, objects, root := newMemoryMeta(), objecttest.NewMem(), t.TempDir()
	manager, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, keys := range [][2]string{{"bad", "bad"}, {"missing.pack", "missing.idx"}} {
		if err := manager.installPair(ctx, keys[0], keys[1], "", root); err == nil {
			t.Fatalf("installed pack pair %q", keys)
		}
	}
	if err := manager.download(ctx, "missing", filepath.Join(root, "missing.pack")); err == nil {
		t.Fatal("downloaded missing object")
	}
	if _, _, err := hashFile(filepath.Join(root, "missing")); err == nil {
		t.Fatal("hashed missing file")
	}
	if err := putFile(ctx, objects, "a/key", filepath.Join(root, "missing")); err == nil {
		t.Fatal("put missing file")
	}
	if err := writeCacheState(filepath.Join(root, "missing-dir"), 1, "main"); err == nil {
		t.Fatal("wrote sequence outside repository")
	}
	fileRoot := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(metadata, objects, filepath.Join(fileRoot, "child")); err == nil {
		t.Fatal("created cache beneath file")
	}
}

func TestRepositoryStagingDiffAndEmptyCompact(t *testing.T) {
	metadata, objects, root := newMemoryMeta(), objecttest.NewMem(), t.TempDir()
	manager, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	file, err := stageInput(root, strings.NewReader("abc"), 3)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_ = os.Remove(file.Name())
	if _, err := stageInput(root, strings.NewReader("abcd"), 3); err == nil {
		t.Fatal("accepted oversized receive")
	}
	updates := refDiff(map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "3", "c": "4"})
	if len(updates) != 3 || updates[0].Name != "a" {
		t.Fatalf("diff %+v", updates)
	}
	if refs, err := manager.Refs(ctx, types.Repo{ID: "missing"}); err != nil || len(refs) != 0 {
		t.Fatalf("refs %+v %v", refs, err)
	}
	if wal, err := manager.WAL(ctx, types.Repo{ID: "missing"}); err != nil || len(wal) != 0 {
		t.Fatalf("wal %+v %v", wal, err)
	}
	if _, err := exec.LookPath("git"); err == nil {
		empty := types.Repo{ID: "repo_empty", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
		if err := manager.Compact(ctx, empty); err == nil {
			t.Fatal("compacted repository without objects")
		}
	}
}
