package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

type localImportPublisher struct {
	manager *Manager
	source  string
}

func (p localImportPublisher) ImportControlled(ctx context.Context, repo types.Repo, spec types.ImportSpec) (string, error) {
	spec.URL = "file://" + p.source
	spec.PinnedAddress = ""
	return p.manager.importRepo(ctx, repo, spec)
}

func (p localImportPublisher) Upgrade(ctx context.Context, repo types.Repo) (types.Repo, error) {
	return p.manager.Upgrade(ctx, repo)
}

func TestImportDiscoversMasterAndRebuilds(t *testing.T) {
	testkit.GitAvailable(t)
	ctx := context.Background()
	source := masterRepository(t)
	metadata, _ := forkFixture(t, ctx)
	objects := objecttest.NewMem()
	manager, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := jobs.NewWithPublisher(metadata, objects, "http://example.test", localImportPublisher{manager: manager, source: source})
	created, err := runner.Import(ctx, "acct", "imports", "master-repo", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	if created.DefaultBranch != "master" {
		t.Fatalf("default branch %q", created.DefaultBranch)
	}
	ns, err := metadata.Namespaces().GetByName(ctx, "acct", "imports")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := metadata.Repos().GetByName(ctx, ns.ID, "master-repo")
	if err != nil || repo.DefaultBranch != "master" {
		t.Fatalf("repo %+v %v", repo, err)
	}
	if err := manager.Evict(repo); err != nil {
		t.Fatal(err)
	}
	assertMasterFile(t, manager, repo)
}

func masterRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testkit.RunGit(t, dir, "init", "-b", "master")
	testkit.RunGit(t, dir, "config", "user.name", "artifacts")
	testkit.RunGit(t, dir, "config", "user.email", "artifacts@test")
	if err := os.WriteFile(filepath.Join(dir, "artifact.txt"), []byte("master\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, dir, "add", "artifact.txt")
	testkit.RunGit(t, dir, "-c", "commit.gpgsign=false", "commit", "-m", "master")
	return dir
}

func assertMasterFile(t *testing.T, manager *Manager, repo types.Repo) {
	t.Helper()
	err := manager.Read(context.Background(), repo, func(store storer.Storer) error {
		ref, err := store.Reference(plumbing.NewBranchReferenceName("master"))
		if err != nil {
			return err
		}
		commit, err := gitobject.GetCommit(store, ref.Hash())
		if err != nil {
			return err
		}
		file, err := commit.File("artifact.txt")
		if err != nil {
			return err
		}
		got, err := file.Contents()
		if err == nil && got != "master\n" {
			return meta.ErrNotFound
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
