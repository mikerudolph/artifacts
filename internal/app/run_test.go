package app

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/repository"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object/fs"
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

func TestCLIProductionPaths(t *testing.T) {
	dsn := testkit.Postgres(t)
	data, cache := t.TempDir(), t.TempDir()
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("ARTIFACTS_STORAGE", "fs")
	t.Setenv("ARTIFACTS_DATA_DIR", data)
	t.Setenv("ARTIFACTS_CACHE_DIR", cache)
	t.Setenv("ARTIFACTS_PUBLIC_URL", "http://127.0.0.1:8080")
	ctx := context.Background()
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(ctx, []string{"token", "create", "--account", "tenant-a"}, out, errb); code != 0 || !strings.HasPrefix(out.String(), "art_api_v1_") {
		t.Fatalf("token create %d %q %q", code, out.String(), errb.String())
	}
	metadata, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closer, ok := metadata.(interface{ Close() }); ok {
			closer.Close()
		}
	}()
	if err := ensureAPIToken(ctx, metadata, "tenant-a", "legacy-token"); err != nil {
		t.Fatal(err)
	}
	if err := ensureAPIToken(ctx, metadata, "tenant-a", "legacy-token"); err != nil {
		t.Fatal(err)
	}
	services := service.New(metadata, time.Now, "http://127.0.0.1:8080")
	created, err := services.CreateRepo(ctx, "tenant-a", "agents", types.CreateRepoInput{Name: "session"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := services.GetRepo(ctx, "tenant-a", "agents", string(created.Name))
	if err != nil {
		t.Fatal(err)
	}
	objects, err := fs.New(data)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := repository.New(metadata, objects, cache)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: "b"}}}); err != nil {
		t.Fatal(err)
	}
	if code := Run(ctx, []string{"compact", "--account", "tenant-a", "--namespace", "agents", "--repo", "session"}, out, errb); code != 0 {
		t.Fatalf("compact %d %q", code, errb.String())
	}
	if code := Run(ctx, []string{"compact", "--account", "tenant-a", "--namespace", "agents", "--repo", "missing"}, out, errb); code != 1 {
		t.Fatalf("missing compact %d", code)
	}
}

func TestCLIListenersAndDevHandler(t *testing.T) {
	dsn := testkit.Postgres(t)
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("ARTIFACTS_STORAGE", "fs")
	t.Setenv("ARTIFACTS_DATA_DIR", t.TempDir())
	t.Setenv("ARTIFACTS_CACHE_DIR", t.TempDir())
	t.Setenv("ARTIFACTS_PUBLIC_URL", "http://127.0.0.1:8080")
	ctx, errb := context.Background(), &bytes.Buffer{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	addr := listener.Addr().String()
	t.Setenv("ARTIFACTS_HTTP_ADDR", addr)
	if code := runServe(ctx, errb); code != 1 {
		t.Fatalf("serve occupied %d", code)
	}
	if code := runDev(ctx, []string{"--addr", addr}, errb); code != 1 {
		t.Fatalf("dev occupied %d", code)
	}
	if code := listen(ctx, addr, http.NewServeMux(), errb); code != 1 {
		t.Fatalf("listen occupied %d", code)
	}
	cfg, err := config.LoadNoAuth()
	if err != nil {
		t.Fatal(err)
	}
	h, err := buildHandler(ctx, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Local artifact repositories") {
		t.Fatalf("dev browser %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/client/v4/accounts/tenant-a/artifacts/namespaces", nil)
	req.Host = "localhost:8080"
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dev REST %d", w.Code)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/git/tenant-a/agents/missing.git/info/refs?service=git-upload-pack", nil)
	req.Host = "[::1]:8080"
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("dev Git %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://attacker.example/", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("rebinding host %d", w.Code)
	}
}

func TestTokenLookupAndMigrateOK(t *testing.T) {
	dsn := testkit.Postgres(t)
	t.Setenv("ARTIFACTS_AUTH", "token")
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
		Auth:     config.Auth{Mode: "token"},
		Storage:  config.Storage{Backend: "s3"},
		Postgres: config.Postgres{DSN: dsn},
		Account:  config.Account{DefaultID: "local"},
	})
	if err == nil {
		t.Fatal("expected s3 open fail")
	}
	cacheFile := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(cacheFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Handler(context.Background(), config.Config{
		HTTP:     config.HTTP{PublicURL: "http://x"},
		Auth:     config.Auth{Mode: "token"},
		Storage:  config.Storage{Backend: "fs", FS: config.FS{Path: t.TempDir()}},
		Cache:    config.Cache{Path: cacheFile},
		Postgres: config.Postgres{DSN: dsn},
		Account:  config.Account{DefaultID: "local"},
	})
	if err == nil {
		t.Fatal("expected cache open fail")
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

func TestDevBuildAndListenerSafetyErrors(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://127.0.0.1:1/none?sslmode=disable")
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("ARTIFACTS_STORAGE", "fs")
	t.Setenv("ARTIFACTS_DATA_DIR", t.TempDir())
	t.Setenv("ARTIFACTS_CACHE_DIR", t.TempDir())
	errbuf := &bytes.Buffer{}
	if code := runDev(context.Background(), []string{"--addr", "127.0.0.1:0"}, errbuf); code != 1 {
		t.Fatalf("build error exit %d", code)
	}
	if code := listenDev(context.Background(), "0.0.0.0:0", http.NewServeMux(), errbuf); code != 2 {
		t.Fatalf("non-loopback listener exit %d", code)
	}
}
