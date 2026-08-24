package service

import (
	"context"
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

func TestCreateRepoImplicitNSAndToken(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0).UTC()
	svc := New(newFake(), func() time.Time { return now }, "http://localhost:8080")
	ctx := context.Background()
	got, err := svc.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: "starter-repo", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != "http://localhost:8080/git/default/starter-repo.git" {
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
