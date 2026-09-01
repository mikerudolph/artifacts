package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceRebasesIndependentPublication(t *testing.T) {
	_, remote := testBrain(t)
	ctx := context.Background()
	one, err := cloneWorkspace(ctx, remote, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = one.Close() }()
	two, err := cloneWorkspace(ctx, remote, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = two.Close() }()
	_, err = one.mutate(ctx, "Add one", func(root string) error {
		if err := writeRepositoryFile(root, "one.txt", []byte("one")); err != nil {
			return err
		}
		_, err := two.mutate(ctx, "Add two", func(other string) error {
			return writeRepositoryFile(other, "two.txt", []byte("two"))
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := cloneWorkspace(ctx, remote, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fresh.Close() }()
	for _, name := range []string{"one.txt", "two.txt"} {
		if _, err := os.Stat(filepath.Join(fresh.dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestWorkspaceFailuresAreSafe(t *testing.T) {
	ctx := context.Background()
	_, err := cloneWorkspace(ctx, filepath.Join(t.TempDir(), "missing.git"), "top-secret")
	if err == nil || strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("clone error=%v", err)
	}
	b, remote := testBrain(t)
	_, err = b.workspace.mutate(ctx, "Fails", func(string) error { return errors.New("apply failed") })
	if err == nil {
		t.Fatal("apply error ignored")
	}
	_, err = b.workspace.mutate(ctx, "No changes", func(string) error { return nil })
	if err == nil {
		t.Fatal("empty commit accepted")
	}
	if err := os.RemoveAll(remote); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.read(ctx, func(string) error { return nil }); err == nil {
		t.Fatal("missing remote accepted")
	}
}

func TestWorkspaceDiscardsUnpublishedCommit(t *testing.T) {
	ctx := context.Background()
	b, remote := testBrain(t)
	if _, err := b.remember(ctx, rememberInput{Content: "published", Kind: "note"}); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(runTestGit(t, b.workspace.dir, "rev-parse", "HEAD"))
	_, err := b.workspace.mutate(ctx, "Must not survive", func(root string) error {
		if err := writeRepositoryFile(root, "doomed.txt", []byte("private content")); err != nil {
			return err
		}
		return os.RemoveAll(remote)
	})
	if err == nil || strings.Contains(err.Error(), "private content") {
		t.Fatalf("push error=%v", err)
	}
	if _, err := os.Stat(filepath.Join(b.workspace.dir, "doomed.txt")); !os.IsNotExist(err) {
		t.Fatalf("unpublished file remained: %v", err)
	}
	if got := strings.TrimSpace(runTestGit(t, b.workspace.dir, "rev-parse", "HEAD")); got != head {
		t.Fatalf("HEAD=%s want %s", got, head)
	}
}
