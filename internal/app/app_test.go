package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestRunHelpAndUnknown(t *testing.T) {
	t.Parallel()
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(context.Background(), nil, out, errb); code != 0 {
		t.Fatal(code)
	}
	if code := Run(context.Background(), []string{"wat"}, out, errb); code != 2 {
		t.Fatal(code)
	}
}

func TestDevAndCompactArgumentSafety(t *testing.T) {
	t.Parallel()
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(context.Background(), []string{"dev", "--addr", "0.0.0.0:8080"}, out, errb); code != 2 {
		t.Fatalf("unsafe dev code %d", code)
	}
	if code := Run(context.Background(), []string{"compact"}, out, errb); code != 2 {
		t.Fatalf("missing compact repo code %d", code)
	}
	if code := Run(context.Background(), []string{"dev", "--bad-flag"}, out, errb); code != 2 {
		t.Fatalf("bad dev flag %d", code)
	}
	if code := Run(context.Background(), []string{"dev", "--allow-remote", "--addr", "0.0.0.0:8080"}, out, errb); code != 2 {
		t.Fatalf("removed remote override %d", code)
	}
	if code := Run(context.Background(), []string{"token", "unknown"}, out, errb); code != 2 {
		t.Fatalf("bad token command %d", code)
	}
	if code := Run(context.Background(), []string{"token", "create", "--bad-flag"}, out, errb); code != 2 {
		t.Fatalf("bad token flag %d", code)
	}
	if !loopbackAddress("127.0.0.1:8080") || !loopbackAddress("[::1]:8080") || loopbackAddress(":8080") || loopbackAddress("bad") {
		t.Fatal("loopback validation")
	}
	srv := newHTTPServer(":0", http.NewServeMux())
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 || srv.IdleTimeout == 0 || srv.MaxHeaderBytes == 0 {
		t.Fatal("missing server resource limits")
	}
}

func TestCLIConfigurationErrors(t *testing.T) {
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("ARTIFACTS_STORAGE", "invalid")
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	ctx := context.Background()
	if runServe(ctx, errb) != 1 || runDev(ctx, nil, errb) != 1 || runMigrate(errb) != 1 {
		t.Fatal("configuration error was not returned")
	}
	if runToken(ctx, []string{"create"}, out, errb) != 1 {
		t.Fatal("token ignored bad configuration")
	}
	if runCompact(ctx, []string{"--repo", "app"}, errb) != 1 {
		t.Fatal("compact ignored bad configuration")
	}
}

func TestTokenAndMigrate(t *testing.T) {
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("ARTIFACTS_API_TOKEN", "")
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	if code := Run(context.Background(), []string{"token"}, out, errb); code != 1 {
		t.Fatal(code)
	}
	t.Setenv("ARTIFACTS_API_TOKEN", "secret")
	t.Setenv("ARTIFACTS_AUTH", "token")
	out.Reset()
	if code := Run(context.Background(), []string{"token"}, out, errb); code != 0 || out.String() == "" {
		t.Fatalf("%d %q", code, out.String())
	}
}

func TestE2E(t *testing.T) {
	testkit.GitAvailable(t)
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	cacheRoot := t.TempDir()
	cfg := config.Config{
		HTTP:     config.HTTP{Addr: ":0", PublicURL: "http://example"},
		Auth:     config.Auth{Mode: "token", APIToken: "control-secret"},
		Storage:  config.Storage{Backend: "fs", FS: config.FS{Path: t.TempDir()}},
		Cache:    config.Cache{Path: cacheRoot},
		Postgres: config.Postgres{DSN: dsn},
		Account:  config.Account{DefaultID: "local"},
	}
	h, err := Handler(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	body := bytes.NewBufferString(`{"name":"app"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/client/v4/accounts/local/artifacts/namespaces/default/repos", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer control-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	var env struct {
		Result struct {
			Token  string `json:"token"`
			Remote string `json:"remote"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	u := srv.URL[len("http://"):]
	remote := "http://x:" + stripExpires(env.Result.Token) + "@" + u + "/git/local/default/app.git"
	src := testkit.TempRepo(t)
	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "large.bin"), bytes.Repeat([]byte("x"), 2<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "add", "README.md", "large.bin")
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "commit", "-m", "init")
	testkit.RunGit(t, src, "remote", "add", "origin", remote)
	testkit.RunGit(t, src, "-c", "protocol.version=1", "push", "origin", "main")
	exerciseRefOnlyPushes(t, src)
	devHandler, err := buildHandler(context.Background(), cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	assertDevRESTRefs(t, devHandler)
	exerciseConcurrentReaders(t, srv.URL, remote, devHandler)
	if err := os.RemoveAll(cacheRoot); err != nil {
		t.Fatal(err)
	}
	assertFileRead(t, srv.URL)
	deleteRepoTwice(t, srv.URL)
	assertDeletedLifecycle(t, dsn, env.Result.Token)
}

func assertDevRESTRefs(t *testing.T, handler http.Handler) {
	t.Helper()
	if err := readDevRESTRefs(handler); err != nil {
		t.Fatal(err)
	}
}

func readDevRESTRefs(handler http.Handler) error {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/client/v4/accounts/local/artifacts/namespaces/default/repos/app/refs", nil)
	req.Host = "localhost"
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return fmt.Errorf("dev REST refs %d %q", w.Code, w.Body.String())
	}
	return nil
}

func exerciseConcurrentReaders(t *testing.T, root, remote string, dev http.Handler) {
	t.Helper()
	checks := []func() error{
		func() error { return cloneLargeRepository(t.TempDir(), remote) },
		func() error { return readLargeREST(root) },
		func() error {
			if err := readDevRESTRefs(dev); err != nil {
				return err
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/local/default/app", nil)
			req.Host = "localhost"
			dev.ServeHTTP(w, req)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "large.bin") {
				return fmt.Errorf("UI read %d %q", w.Code, w.Body.String())
			}
			return nil
		},
	}
	errs := make(chan error, len(checks))
	for _, check := range checks {
		go func(run func() error) { errs <- run() }(check)
	}
	for range checks {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func cloneLargeRepository(dest, remote string) error {
	cmd := exec.Command("git", "-c", "protocol.version=1", "clone", remote, filepath.Join(dest, "clone")) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone: %w: %s", err, out)
	}
	return nil
}

func readLargeREST(root string) error {
	req, _ := http.NewRequest(http.MethodGet, root+"/client/v4/accounts/local/artifacts/namespaces/default/repos/app/file?ref=main&path=large.bin", nil)
	req.Header.Set("Authorization", "Bearer control-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || n != 2<<20 {
		return errors.New("large REST read failed")
	}
	return nil
}

func exerciseRefOnlyPushes(t *testing.T, src string) {
	t.Helper()
	first := strings.TrimSpace(testkit.RunGit(t, src, "rev-parse", "HEAD"))
	testkit.RunGit(t, src, "push", "origin", "main:refs/heads/feature")
	testkit.RunGit(t, src, "push", "origin", ":refs/heads/feature")
	if err := os.WriteFile(filepath.Join(src, "second.txt"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, src, "-c", "core.hooksPath="+t.TempDir(), "-c", "commit.gpgsign=false", "add", "second.txt")
	testkit.RunGit(t, src, "-c", "core.hooksPath="+t.TempDir(), "-c", "commit.gpgsign=false", "commit", "-m", "second")
	testkit.RunGit(t, src, "push", "origin", "main")
	testkit.RunGit(t, src, "push", "--force", "origin", first+":refs/heads/main")
}

func assertFileRead(t *testing.T, root string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, root+"/client/v4/accounts/local/artifacts/namespaces/default/repos/app/file?ref=main&path=README.md", nil)
	req.Header.Set("Authorization", "Bearer control-secret")
	fileResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(fileResp.Body)
	_ = fileResp.Body.Close()
	if fileResp.StatusCode != 200 || string(b) != "hi\n" {
		t.Fatalf("file %d %q", fileResp.StatusCode, b)
	}
}

func deleteRepoTwice(t *testing.T, root string) {
	t.Helper()
	deleteURL := root + "/client/v4/accounts/local/artifacts/namespaces/default/repos/app"
	for range 2 {
		req, _ := http.NewRequest(http.MethodDelete, deleteURL, nil)
		req.Header.Set("Authorization", "Bearer control-secret")
		deleted, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = deleted.Body.Close()
		if deleted.StatusCode != http.StatusAccepted {
			t.Fatalf("delete %d", deleted.StatusCode)
		}
	}
}

func assertDeletedLifecycle(t *testing.T, dsn, credential string) {
	t.Helper()
	ctx := context.Background()
	metadata, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closer, ok := metadata.(interface{ Close() }); ok {
			closer.Close()
		}
	}()
	ns, err := metadata.Namespaces().GetByName(ctx, "local", "default")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := metadata.Repos().GetByName(ctx, ns.ID, "app")
	if err != nil || repo.Status != types.RepoDeleted {
		t.Fatalf("deleted repo %+v %v", repo, err)
	}
	secret, _, err := auth.ParseRepo(credential)
	if err != nil {
		t.Fatal(err)
	}
	token, err := metadata.RepoTokens().GetByHash(ctx, auth.HashRepo(secret))
	if err != nil || token.State != types.TokenRevoked {
		t.Fatalf("revoked token %+v %v", token, err)
	}
	jobs, err := metadata.Jobs().ListByRepo(ctx, repo.ID)
	if err != nil || len(jobs) == 0 || jobs[0].Status != types.JobSucceeded {
		t.Fatalf("delete jobs %+v %v", jobs, err)
	}
}

func stripExpires(tok string) string {
	for i := 0; i < len(tok); i++ {
		if tok[i] == '?' {
			return tok[:i]
		}
	}
	return tok
}
