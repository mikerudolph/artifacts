package app

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestRunMigrateAndHandlerErrors(t *testing.T) {
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("DATABASE_URL", "postgres://127.0.0.1:1/none?sslmode=disable")
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(context.Background(), []string{"migrate"}, out, errb); code != 1 {
		t.Fatalf("migrate bad dsn: %d", code)
	}
	if code := Run(context.Background(), []string{"serve"}, out, errb); code != 1 {
		t.Fatalf("serve bad: %d", code)
	}
	_, err := Handler(context.Background(), config.Config{
		HTTP:     config.HTTP{PublicURL: "http://x"},
		Auth:     config.Auth{Mode: "none"},
		Storage:  config.Storage{Backend: "fs", FS: config.FS{Path: t.TempDir()}},
		Postgres: config.Postgres{DSN: "postgres://127.0.0.1:1/none?sslmode=disable"},
		Account:  config.Account{DefaultID: "local"},
	})
	if err == nil {
		t.Fatal("expected handler error")
	}
	if _, err := openObjects(context.Background(), config.Config{Storage: config.Storage{Backend: "s3"}}); err == nil {
		t.Fatal("expected s3 error")
	}
}

func TestTokenLookupAndMigrateOK(t *testing.T) {
	dsn := testkit.Postgres(t)
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("DATABASE_URL", dsn)
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(context.Background(), []string{"migrate"}, out, errb); code != 0 {
		t.Fatalf("migrate: %d %s", code, errb.String())
	}
	st, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Accounts().Ensure(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	ns, err := st.Namespaces().Create(context.Background(), types.Namespace{AccountID: "local", Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.Repos().Create(context.Background(), types.Repo{NamespaceID: ns.ID, Name: "app", DefaultBranch: "main", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	plain, hash, id, exp, err := auth.MintRepo(types.ScopeRead, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.RepoTokens().Create(context.Background(), types.RepoToken{
		ID: id, RepoID: repo.ID, Hash: hash, Scope: types.ScopeRead, State: types.TokenActive, ExpiresAt: exp,
	})
	if err != nil {
		t.Fatal(err)
	}
	l := tokenLookup{meta: st}
	got, err := l.Lookup(context.Background(), "default", "app", plain)
	if err != nil || got != types.ScopeRead {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := l.Lookup(context.Background(), "default", "app", "nope"); err == nil {
		t.Fatal("expected parse error")
	}
	_, err = st.RepoTokens().Create(context.Background(), types.RepoToken{
		ID: "revokedtoken01", RepoID: repo.ID, Hash: auth.HashRepo("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Scope: types.ScopeRead, State: types.TokenRevoked, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Lookup(context.Background(), "", "", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("expected revoked")
	}
	if _, err := l.Lookup(context.Background(), "", "", "cccccccccccccccccccccccccccccccccccccccc"); err == nil {
		t.Fatal("expected missing hash")
	}
}

func TestHandlerOpenObjectsFail(t *testing.T) {
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	_, err := Handler(context.Background(), config.Config{
		HTTP:     config.HTTP{PublicURL: "http://x"},
		Auth:     config.Auth{Mode: "none"},
		Storage:  config.Storage{Backend: "s3"},
		Postgres: config.Postgres{DSN: dsn},
		Account:  config.Account{DefaultID: "local"},
	})
	if err == nil {
		t.Fatal("expected s3 open fail")
	}
	t.Setenv("ARTIFACTS_AUTH", "token")
	t.Setenv("ARTIFACTS_API_TOKEN", "")
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(context.Background(), []string{"serve"}, out, errb); code != 1 {
		t.Fatal(code)
	}
	if code := Run(context.Background(), []string{"migrate"}, out, errb); code != 1 {
		t.Fatal(code)
	}
	if code := Run(context.Background(), []string{"token"}, out, errb); code != 1 {
		t.Fatal(code)
	}
}
