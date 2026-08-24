package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/testkit"
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
	cfg := config.Config{
		HTTP:     config.HTTP{Addr: ":0", PublicURL: "http://example"},
		Auth:     config.Auth{Mode: "none"},
		Storage:  config.Storage{Backend: "fs", FS: config.FS{Path: t.TempDir()}},
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
	resp, err := http.Post(srv.URL+"/client/v4/accounts/local/artifacts/namespaces/default/repos", "application/json", body)
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
	remote := "http://x:" + stripExpires(env.Result.Token) + "@" + u + "/git/default/app.git"
	src := testkit.TempRepo(t)
	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "add", "README.md")
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "commit", "-m", "init")
	testkit.RunGit(t, src, "remote", "add", "origin", remote)
	testkit.RunGit(t, src, "-c", "protocol.version=1", "push", "origin", "main")
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/client/v4/accounts/local/artifacts/namespaces/default/repos/app/file?ref=main&path=README.md", nil)
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

func stripExpires(tok string) string {
	for i := 0; i < len(tok); i++ {
		if tok[i] == '?' {
			return tok[:i]
		}
	}
	return tok
}
