package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestIncrementalCommitPreservesGitObjects(t *testing.T) {
	ctx := context.Background()
	metadata := newMemoryMeta()
	m, err := New(metadata, objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "changes", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	seed, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "input", Content: "keep"}, {Path: "output", Content: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = seed.Sequence
	branch, err := m.Commit(ctx, repo, types.CommitInput{Branch: "work", Deletes: []string{"output"}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = branch.Sequence
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	git := func(args ...string) string {
		t.Helper()
		out, err := runGit(ctx, nil, append([]string{"--git-dir=" + path}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := git("show", branch.SHA+":input"); got != "keep" {
		t.Fatal(got)
	}
	if got := git("rev-parse", branch.SHA+"^"); got != seed.SHA {
		t.Fatal("new branch lost history")
	}
	empty, err := m.Commit(ctx, repo, types.CommitInput{Branch: "work", ExpectedHead: &branch.SHA, Deletes: []string{"input"}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = empty.Sequence
	if got := git("ls-tree", empty.SHA); got != "" {
		t.Fatal("last file not deleted")
	}
	_, err = m.Commit(ctx, repo, types.CommitInput{ExpectedHead: &branch.SHA, Files: []types.CommitFile{{Path: "output", Content: "bad"}}})
	var conflict *types.HeadConflict
	if !errors.As(err, &conflict) || conflict.Current != seed.SHA {
		t.Fatalf("conflict: %v", err)
	}
	pinned, err := m.Commit(ctx, repo, types.CommitInput{Branch: "pinned", Base: seed.SHA, Files: []types.CommitFile{{Path: "output", Content: "done"}}})
	if err != nil {
		t.Fatal(err)
	}
	repo.WALSequence = pinned.Sequence
	if got := git("show", pinned.SHA+":input"); got != "keep" {
		t.Fatal(got)
	}
	if _, err = m.Commit(ctx, repo, types.CommitInput{Branch: "pinned", Base: seed.SHA, Files: []types.CommitFile{{Path: "x"}}}); err == nil {
		t.Fatal("base accepted on existing branch")
	}
}
