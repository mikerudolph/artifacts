package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/types"
)

func TestIncrementalIndexPreservesModesAndBinary(t *testing.T) {
	ctx := context.Background()
	gitDir := filepath.Join(t.TempDir(), "repo.git")
	if _, err := runGit(ctx, nil, "init", "--bare", gitDir); err != nil {
		t.Fatal(err)
	}
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(t.TempDir(), "index")}
	entries := []struct{ name, mode, content string }{
		{"executable", "100755", "#!/bin/sh\necho hello\n"},
		{"link", "120000", "/outside"},
		{"binary", "100644", "\x00\xff\x01"},
		{"report", "100644", "draft"},
	}
	for _, entry := range entries {
		blob, err := runGit(ctx, strings.NewReader(entry.content), "--git-dir="+gitDir, "hash-object", "-w", "--stdin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "update-index", "--add", "--cacheinfo", entry.mode, strings.TrimSpace(string(blob)), entry.name); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "write-tree")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := createCommit(ctx, gitDir, strings.TrimSpace(string(tree)), "", types.CommitInput{})
	if err != nil {
		t.Fatal(err)
	}
	next, err := writeCommit(ctx, gitDir, t.TempDir(), parent, types.CommitInput{Files: []types.CommitFile{{Path: "report", Content: "done"}}})
	if err != nil {
		t.Fatal(err)
	}
	assertUnchangedEntries(t, gitDir, parent, next, []string{"executable", "link", "binary"})
	if _, err := writeCommit(ctx, gitDir, t.TempDir(), parent, types.CommitInput{Files: []types.CommitFile{{Path: "link/escape", Content: "bad"}}}); err == nil {
		t.Fatal("write beneath symlink accepted")
	}
	next, err = writeCommit(ctx, gitDir, t.TempDir(), parent, types.CommitInput{Files: []types.CommitFile{{Path: "executable", Content: "new program"}}})
	if err != nil {
		t.Fatal(err)
	}
	mode, err := runGit(ctx, nil, "--git-dir="+gitDir, "ls-tree", next, "--", "executable")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(mode), "100755 ") {
		t.Fatal("executable mode lost on update")
	}
}

func assertUnchangedEntries(t *testing.T, gitDir, parent, next string, names []string) {
	t.Helper()
	ctx := context.Background()
	for _, name := range names {
		before, err := runGit(ctx, nil, "--git-dir="+gitDir, "ls-tree", parent, "--", name)
		if err != nil {
			t.Fatal(err)
		}
		after, err := runGit(ctx, nil, "--git-dir="+gitDir, "ls-tree", next, "--", name)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("untouched %s changed", name)
		}
	}
}
