package jobs

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestImportAndForkErrors(t *testing.T) {
	r := testRunner(t)
	ctx := context.Background()
	if _, err := r.Import(ctx, "local", "-bad", "x", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"}); err == nil {
		t.Fatal("ns")
	}
	if _, err := r.Import(ctx, "local", "default", "-x", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"}); err == nil {
		t.Fatal("name")
	}
	if _, err := r.Import(ctx, "local", "default", "gone", types.ImportRepoInput{URL: "http://127.0.0.1:1/no.git"}); err != ErrInvalidURL {
		t.Fatalf("upstream %v", err)
	}
	if _, err := r.Fork(ctx, "local", "default", "missing", types.ForkRepoInput{Name: "d"}); err == nil {
		t.Fatal("missing src")
	}
}

func TestCloneErrorMapAndLookup(t *testing.T) {
	r := testRunner(t)
	ctx := context.Background()
	if mapCloneErr(nil) != nil {
		t.Fatal("nil")
	}
	if mapCloneErr(transport.ErrAuthenticationRequired) != ErrRemoteAuth {
		t.Fatal("auth")
	}
	if mapCloneErr(errString("repository not found")) != ErrInvalidURL {
		t.Fatal("not found")
	}
	if mapCloneErr(errString("invalid remote")) != ErrInvalidURL {
		t.Fatal("invalid")
	}
	if firstNonEmpty("", "d") != "d" || firstNonEmpty("a", "d") != "a" {
		t.Fatal("first")
	}
	if _, _, err := r.lookup(ctx, "local", "-n", "x"); err == nil {
		t.Fatal("lookup ns")
	}
	if err := r.meta.Accounts().Ensure(ctx, "local"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ensureNS(ctx, "local", "default"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.lookup(ctx, "local", "default", "-r"); err == nil {
		t.Fatal("lookup repo")
	}
	if _, _, err := r.lookup(ctx, "local", "default", "missing"); err == nil {
		t.Fatal("lookup missing repo")
	}
	if err := validateImportURL(ctx, "https://does-not-exist.invalid/repo.git"); err != ErrUpstream {
		t.Fatalf("dns failure %v", err)
	}
}

func TestValidateImportURLSecurity(t *testing.T) {
	t.Parallel()
	cases := []string{
		"", "http://example.com/repo.git", "https://user:pass@example.com/repo.git",
		"https://localhost/repo.git", "https://127.0.0.1/repo.git", "https://[::1]/repo.git",
		"https://169.254.169.254/latest/meta-data", "https://10.0.0.1/repo.git",
		"https://100.64.0.1/repo.git", "https://198.18.0.1/repo.git",
		"https://224.0.0.1/repo.git", "https://240.0.0.1/repo.git",
		"https://[2001:db8::1]/repo.git", "https://[ff02::1]/repo.git", "https://[fec0::1]/repo.git",
	}
	for _, raw := range cases {
		if err := validateImportURL(context.Background(), raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if err := validateImportURL(context.Background(), "https://93.184.216.34/repo.git"); err != nil {
		t.Fatalf("public literal: %v", err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestDeleteExisting(t *testing.T) {
	r := testRunner(t)
	ctx := context.Background()
	_ = r.meta.Accounts().Ensure(ctx, "local")
	ns, err := r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: "local", Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.meta.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "gone", DefaultBranch: "main", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, "local", "default", "gone"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMissing(t *testing.T) {
	r := testRunner(t)
	if err := r.Delete(context.Background(), "local", "default", "nope"); err == nil {
		t.Fatal("expected missing")
	}
}

func TestForkBadName(t *testing.T) {
	r := testRunner(t)
	ctx := context.Background()
	_ = r.meta.Accounts().Ensure(ctx, "local")
	ns, err := r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: "local", Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.meta.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "src", DefaultBranch: "main", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "-bad"}); err == nil {
		t.Fatal("expected name")
	}
	src, _ := r.meta.Repos().GetByName(ctx, ns.ID, "src")
	src.Status = types.RepoForking
	_, _ = r.meta.Repos().Update(ctx, src)
	if _, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "dst"}); err != ErrBusy {
		t.Fatalf("busy %v", err)
	}
	if descPtr("") != nil || descPtr("x") == nil {
		t.Fatal("descPtr")
	}
}
