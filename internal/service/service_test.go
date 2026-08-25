package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestRemoteURL(t *testing.T) {
	t.Parallel()
	got := RemoteURL("http://localhost:8080/", "default", "app")
	if got != "http://localhost:8080/git/default/app.git" {
		t.Fatal(got)
	}
}

type fixedIssuer struct{}

func (fixedIssuer) Issue(context.Context, types.RepoID, types.Scope, int) (types.CreateTokenResult, error) {
	return types.CreateTokenResult{Plaintext: "fixed"}, nil
}

type failedIssuer struct{}

func (failedIssuer) Issue(context.Context, types.RepoID, types.Scope, int) (types.CreateTokenResult, error) {
	return types.CreateTokenResult{}, errors.New("credential store unavailable")
}

type failedRefStore struct{ *fakeStore }

func (s failedRefStore) Refs() meta.Refs { return failedRefs{Refs: s.fakeStore.Refs()} }

type failedRefs struct{ meta.Refs }

func (failedRefs) CompareAndSwap(context.Context, types.RepoID, string, string, string) error {
	return errors.New("ref store unavailable")
}

func TestNewWithIssuer(t *testing.T) {
	services := NewWithIssuer(newFake(), nil, "http://x", fixedIssuer{})
	result, err := services.CreateRepo(context.Background(), "local", "default", types.CreateRepoInput{Name: "app"})
	if err != nil || result.Token != "fixed" {
		t.Fatalf("%+v %v", result, err)
	}
	if NewWithIssuer(newFake(), nil, "http://x", nil).issuer == nil {
		t.Fatal("default issuer missing")
	}
}

func TestCreateCredentialFailureIsNotReady(t *testing.T) {
	store := newFake()
	services := NewWithIssuer(store, nil, "http://x", failedIssuer{})
	if _, err := services.CreateRepo(context.Background(), "local", "default", types.CreateRepoInput{Name: "orphan"}); err == nil {
		t.Fatal("expected credential failure")
	}
	ns, err := store.Namespaces().GetByName(context.Background(), "local", "default")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repos().GetByName(context.Background(), ns.ID, "orphan")
	if err != nil || repo.Status != types.RepoFailed || repo.Failure == "" {
		t.Fatalf("repo %+v %v", repo, err)
	}
}

func TestCreateRefFailureIsNotReady(t *testing.T) {
	store := newFake()
	services := New(failedRefStore{fakeStore: store}, nil, "http://x")
	if _, err := services.CreateRepo(context.Background(), "local", "default", types.CreateRepoInput{Name: "no-head"}); err == nil {
		t.Fatal("expected ref failure")
	}
	ns, _ := store.Namespaces().GetByName(context.Background(), "local", "default")
	repo, err := store.Repos().GetByName(context.Background(), ns.ID, "no-head")
	if err != nil || repo.Status != types.RepoFailed {
		t.Fatalf("repo %+v %v", repo, err)
	}
}

func TestCreateRepoImplicitNSAndToken(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0).UTC()
	svc := New(newFake(), func() time.Time { return now }, "http://localhost:8080")
	ctx := context.Background()
	got, err := svc.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: "starter-repo", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != "http://localhost:8080/git/local/default/starter-repo.git" {
		t.Fatal(got.Remote)
	}
	if got.Token == "" || got.DefaultBranch != "main" || got.Description == nil {
		t.Fatalf("%+v", got)
	}
	repo, err := svc.GetRepo(ctx, "local", "default", "starter-repo")
	if err != nil || repo.Remote != got.Remote {
		t.Fatalf("%+v %v", repo, err)
	}
	if _, err := svc.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: "starter-repo"}); !meta.IsAlreadyExists(err) {
		t.Fatalf("dup: %v", err)
	}
}

func TestDefaultBranchUpdateMovesSymbolicHead(t *testing.T) {
	store := newFake()
	services := New(store, nil, "http://x")
	ctx := context.Background()
	created, err := services.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: "branches", DefaultBranch: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	next := "release"
	repo, err := services.UpdateRepo(ctx, "local", "default", "branches", types.UpdateRepoInput{DefaultBranch: &next})
	if err != nil || repo.DefaultBranch != next {
		t.Fatalf("repo %+v %v", repo, err)
	}
	head, err := store.Refs().Get(ctx, created.ID, "HEAD")
	if err != nil || head.SHA != "ref:refs/heads/release" {
		t.Fatalf("HEAD %+v %v", head, err)
	}
}

func TestNamespaceCRUD(t *testing.T) {
	t.Parallel()
	svc := New(newFake(), nil, "http://x")
	ctx := context.Background()
	ns, err := svc.CreateNamespace(ctx, "local", "prod", types.JurisdictionEU)
	if err != nil || ns.Name != "prod" {
		t.Fatalf("%+v %v", ns, err)
	}
	got, err := svc.GetNamespace(ctx, "local", "prod")
	if err != nil || got.ID != ns.ID {
		t.Fatalf("%+v %v", got, err)
	}
	list, _, err := svc.ListNamespaces(ctx, "local", types.CursorPage{})
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
	if _, err := svc.CreateNamespace(ctx, "local", "-bad", ""); err == nil {
		t.Fatal("expected name error")
	}
	if _, err := svc.CreateNamespace(ctx, "local", "ok", "apac"); err == nil {
		t.Fatal("expected jurisdiction error")
	}
}

func TestListDeleteToken(t *testing.T) {
	t.Parallel()
	svc := New(newFake(), time.Now, "http://x")
	ctx := context.Background()
	created, err := svc.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: "app"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := svc.CreateToken(ctx, "local", "default", types.CreateTokenInput{Repo: "app", Scope: types.ScopeRead, TTL: 3600})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Scope != types.ScopeRead || !strings.Contains(tok.Plaintext, "art_v1_") {
		t.Fatalf("%+v", tok)
	}
	list, info, err := svc.ListTokens(ctx, "local", "default", "app", types.TokenActive, types.OffsetPage{})
	if err != nil || len(list) < 2 || info.TotalCount < 2 {
		t.Fatalf("%v %+v %v", list, info, err)
	}
	if err := svc.RevokeToken(ctx, "local", "default", tok.ID); err != nil {
		t.Fatal(err)
	}
	id, err := svc.DeleteRepo(ctx, "local", "default", "app")
	if err != nil || id != created.ID {
		t.Fatalf("%s %v", id, err)
	}
	repos, _, err := svc.ListRepos(ctx, "local", "default", meta.ListReposOpts{})
	if err != nil || len(repos) != 1 || repos[0].Status != types.RepoDeleting {
		t.Fatalf("%+v %v", repos, err)
	}
}

func TestServiceErrors(t *testing.T) {
	t.Parallel()
	svc := New(newFake(), time.Now, "http://x")
	ctx := context.Background()
	if _, err := svc.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: ""}); err == nil {
		t.Fatal("expected name error")
	}
	if _, err := svc.GetRepo(ctx, "local", "missing", "app"); err == nil {
		t.Fatal("expected missing ns")
	}
	if _, err := svc.CreateToken(ctx, "local", "default", types.CreateTokenInput{Repo: "app", TTL: 1}); err == nil {
		t.Fatal("expected ttl or missing")
	}
	if _, err := svc.GetNamespace(ctx, "local", "-x"); err == nil {
		t.Fatal("expected parse")
	}
	if err := svc.RevokeToken(ctx, "local", "-x", "id"); err == nil {
		t.Fatal("expected ns parse")
	}
	if _, err := svc.CreateRepo(ctx, "local", "ok", types.CreateRepoInput{Name: "app", DefaultBranch: string(make([]byte, 101))}); err == nil {
		t.Fatal("expected branch error")
	}
	if _, _, err := svc.ListRepos(ctx, "local", "missing", meta.ListReposOpts{}); err == nil {
		t.Fatal("expected missing ns")
	}
	if _, err := svc.DeleteRepo(ctx, "local", "missing", "app"); err == nil {
		t.Fatal("expected missing delete")
	}
	if _, err := svc.CreateToken(ctx, "local", "default", types.CreateTokenInput{Repo: "app", Scope: "admin"}); err == nil {
		t.Fatal("expected scope")
	}
	if _, _, err := svc.ListTokens(ctx, "local", "default", "nope", types.TokenActive, types.OffsetPage{}); err == nil {
		t.Fatal("expected missing tokens repo")
	}
}
