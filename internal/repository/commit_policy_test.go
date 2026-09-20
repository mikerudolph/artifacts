package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestRepositoryCredentialCannotBootstrapReadOnly(t *testing.T) {
	m, err := New(newMemoryMeta(), objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "readonly", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady, ReadOnly: true}
	input := types.CommitInput{RepositoryCredential: true, Files: []types.CommitFile{{Path: "file", Content: "content"}}}
	if _, err := m.Commit(context.Background(), repo, input); !errors.Is(err, types.ErrForbidden) {
		t.Fatalf("repository credential bypassed read-only state: %v", err)
	}
	input.RepositoryCredential = false
	if _, err := m.Commit(context.Background(), repo, input); err != nil {
		t.Fatalf("control bootstrap failed: %v", err)
	}
}

func TestCommitDeletesOnlyExactFiles(t *testing.T) {
	ctx := context.Background()
	m, err := New(newMemoryMeta(), objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "delete", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	seed, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "dir/file", Content: "keep"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = seed.Sequence
	result, err := m.Commit(ctx, repo, types.CommitInput{Deletes: []string{"dir", "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	content, err := runGit(ctx, nil, "--git-dir="+path, "show", result.SHA+":dir/file")
	if err != nil || string(content) != "keep" {
		t.Fatalf("directory prefix deleted child: %v", err)
	}
}
