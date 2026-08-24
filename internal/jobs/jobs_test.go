package jobs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func testRunner(t *testing.T) (*Runner, object.Store) {
	t.Helper()
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	st, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := st.(interface{ Close() }); ok {
			c.Close()
		}
	})
	objs := objecttest.NewMem()
	return New(st, objs, "http://example.test"), objs
}

func TestForkAndDelete(t *testing.T) {
	r, objs := testRunner(t)
	ctx := context.Background()
	if err := r.meta.Accounts().Ensure(ctx, "local"); err != nil {
		t.Fatal(err)
	}
	ns, err := r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: "local", Name: "default", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	src, err := r.meta.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "src", DefaultBranch: "main", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	key := object.LooseObjectKey("local", string(src.ID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := objs.Put(ctx, key, bytes.NewReader([]byte("x")), 1); err != nil {
		t.Fatal(err)
	}
	if err := r.meta.Refs().CompareAndSwap(ctx, src.ID, "refs/heads/main", "", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "dst", DefaultBranchOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Objects != 1 || got.Token == "" {
		t.Fatalf("%+v", got)
	}
	dst, err := r.meta.Repos().GetByName(ctx, ns.ID, "dst")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := r.meta.Refs().Get(ctx, dst.ID, "refs/heads/main")
	if err != nil || ref.SHA == "" {
		t.Fatalf("%v %v", ref, err)
	}
	if _, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "dst"}); err == nil {
		t.Fatal("expected dup fork")
	}
	if err := r.Delete(ctx, "local", "default", "dst"); err != nil {
		t.Fatal(err)
	}
}

func TestImportLocalAndErrors(t *testing.T) {
	r, _ := testRunner(t)
	ctx := context.Background()
	if _, err := r.Import(ctx, "local", "default", "x", types.ImportRepoInput{}); err != ErrInvalidURL {
		t.Fatalf("empty url: %v", err)
	}
	src := testkit.TempRepo(t)
	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f"), []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.GitAvailable(t)
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "add", "f")
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "commit", "-m", "c")
	got, err := r.Import(ctx, "local", "default", "mirror", types.ImportRepoInput{URL: "file://" + src, Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote == "" || got.Token == "" {
		t.Fatalf("%+v", got)
	}
	if _, err := r.Import(ctx, "local", "default", "mirror", types.ImportRepoInput{URL: "file://" + src}); err == nil {
		t.Fatal("expected dup import")
	}
}

func TestBusy(t *testing.T) {
	if err := busy(types.RepoImporting); err != ErrBusy {
		t.Fatal(err)
	}
	if err := busy(types.RepoReady); err != nil {
		t.Fatal(err)
	}
}
