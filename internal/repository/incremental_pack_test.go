package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

type countedObjects struct {
	*objecttest.Mem
	fullPacks, rangeBytes int64
}

func (s *countedObjects) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if strings.HasSuffix(key, ".pack") {
		s.fullPacks++
	}
	return s.Mem.Get(ctx, key)
}
func (s *countedObjects) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	r, err := s.Mem.GetRange(ctx, key, offset, length)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(r)
	s.rangeBytes += int64(len(b))
	return io.NopCloser(bytes.NewReader(b)), err
}

func TestIncrementalPackAndSelectiveColdRead(t *testing.T) {
	ctx := context.Background()
	metadata := newMemoryMeta()
	objects := &countedObjects{Mem: objecttest.NewMem()}
	m, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "selective", AccountID: "acct", DefaultBranch: "main", StorageVersion: 2}

	data := make([]byte, 650000)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	first, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "large", Content: base64.StdEncoding.EncodeToString(data)}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = first.Sequence
	second, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "small", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = second.Sequence
	packs := metadata.packs[repo.ID]
	if packs[1].Size >= packs[0].Size/10 {
		t.Fatalf("second pack %d bytes, first %d", packs[1].Size, packs[0].Size)
	}
	if err := m.Evict(repo); err != nil {
		t.Fatal(err)
	}
	objects.fullPacks, objects.rangeBytes = 0, 0
	readRemoteFile(t, m, repo, second.SHA, "small", "hello")
	if objects.fullPacks != 0 || objects.rangeBytes >= packs[0].Size {
		t.Fatalf("read cost: full packs=%d range bytes=%d", objects.fullPacks, objects.rangeBytes)
	}
	if _, err := os.Stat(filepath.Join(m.root, "acct", "selective", "HEAD")); !os.IsNotExist(err) {
		t.Fatal("content read materialized Git repository")
	}

	readRemoteFile(t, m, repo, second.SHA, "large", base64.StdEncoding.EncodeToString(data))
	if err := m.Compact(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := m.Evict(repo); err != nil {
		t.Fatal(err)
	}
	readRemoteFile(t, m, repo, first.SHA, "large", base64.StdEncoding.EncodeToString(data))
}

func readRemoteFile(t *testing.T, m *Manager, repo types.Repo, sha, path, want string) {
	t.Helper()
	err := m.ReadContent(context.Background(), repo, func(s storer.Storer) error {
		commit, err := gitobject.GetCommit(s, plumbing.NewHash(sha))
		if err != nil {
			return err
		}
		file, err := commit.File(path)
		if err != nil {
			return err
		}
		got, err := file.Contents()
		if err != nil {
			return err
		}
		if got != want {
			t.Fatalf("unexpected file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoteForkWatermarkAndIndexRepair(t *testing.T) {
	ctx := context.Background()
	metadata := newMemoryMeta()
	objects := objecttest.NewMem()
	m, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "source", AccountID: "acct", DefaultBranch: "main", StorageVersion: 2}
	first, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: "before"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = first.Sequence
	child := types.Repo{ID: "child", AccountID: "acct", DefaultBranch: "main", StorageVersion: 2}
	metadata.forks[child.ID] = types.ForkLineage{RepoID: child.ID, ParentRepoID: repo.ID, ParentSequence: 1}
	metadata.refs[child.ID] = map[string]string{"refs/heads/main": first.SHA}
	second, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: "after"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = second.Sequence
	if err := m.Compact(ctx, repo); err != nil {
		t.Fatal(err)
	}
	readRemoteFile(t, m, child, first.SHA, "a", "before")
	err = m.ReadContent(ctx, child, func(s storer.Storer) error {
		if s.HasEncodedObject(plumbing.NewHash(second.SHA)) == nil {
			t.Fatal("fork crossed watermark")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(m.root, "acct", "child.indexes", filepath.Base(metadata.packs[repo.ID][0].IndexKey))
	if err := os.WriteFile(indexPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	readRemoteFile(t, m, child, first.SHA, "a", "before")
}
