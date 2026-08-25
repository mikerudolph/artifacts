package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func testRunner(t *testing.T) *Runner {
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
	return New(st, objs, "http://example.test")
}

func TestForkAndDelete(t *testing.T) {
	r := testRunner(t)
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
	if err := r.meta.Refs().CompareAndSwap(ctx, src.ID, "refs/heads/main", "", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "dst", DefaultBranchOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Objects != 0 || got.Token == "" {
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
	r := testRunner(t)
	ctx := context.Background()
	if _, err := r.Import(ctx, "local", "default", "x", types.ImportRepoInput{}); err != ErrInvalidURL {
		t.Fatalf("empty url: %v", err)
	}
	r.imports = successfulImport{}
	got, err := r.Import(ctx, "local", "default", "mirror", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote == "" || got.Token == "" {
		t.Fatalf("%+v", got)
	}
	if _, err := r.Import(ctx, "local", "default", "mirror", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"}); err == nil {
		t.Fatal("expected dup import")
	}
}

type successfulImport struct{}

func (successfulImport) ImportControlled(_ context.Context, _ types.Repo, spec types.ImportSpec) (string, error) {
	return firstNonEmpty(spec.Branch, types.DefaultBranch), nil
}

type failedImport struct{}

func (failedImport) ImportControlled(context.Context, types.Repo, types.ImportSpec) (string, error) {
	return "", errors.New("clone failed")
}

func TestImportFailureIsDurable(t *testing.T) {
	r := testRunner(t)
	r.imports = failedImport{}
	_, err := r.Import(context.Background(), "local", "agents", "failed", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"})
	if err == nil {
		t.Fatal("expected import error")
	}
	ns, err := r.meta.Namespaces().GetByName(context.Background(), "local", "agents")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := r.meta.Repos().GetByName(context.Background(), ns.ID, "failed")
	if err != nil || repo.Status != types.RepoFailed || repo.Failure == "" {
		t.Fatalf("repo %+v %v", repo, err)
	}
	jobs, err := r.List(context.Background(), repo.ID)
	if err != nil || len(jobs) != 1 || jobs[0].Status != types.JobFailed {
		t.Fatalf("jobs %+v %v", jobs, err)
	}
}

func TestImportFailsClosedWithoutPublisher(t *testing.T) {
	runner := testRunner(t)
	_, err := runner.Import(context.Background(), "local", "default", "closed", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("no publisher error %v", err)
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
