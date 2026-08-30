package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mikerudolph/artifacts/internal/app"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/testkit"
)

func TestCoreIntegration(t *testing.T) {
	testkit.GitAvailable(t)
	dsn := testkit.Postgres(t)
	srv := httptest.NewUnstartedServer(nil)
	publicURL := "http://" + srv.Listener.Addr().String()
	cfg := config.Config{
		HTTP:     config.HTTP{Addr: ":0", PublicURL: publicURL},
		Auth:     config.Auth{Mode: "token", APIToken: "control-secret"},
		Storage:  config.Storage{Backend: "fs", FS: config.FS{Path: t.TempDir()}},
		Cache:    config.Cache{Path: filepath.Join(t.TempDir(), "cache")},
		Postgres: config.Postgres{DSN: dsn},
		Account:  config.Account{DefaultID: "local"},
	}
	handler, err := app.Handler(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = handler
	srv.Start()
	defer srv.Close()
	h := newHarness(configuration{root: srv.URL, account: "local", token: "control-secret"})
	evidence := t.TempDir()
	t.Setenv("ARTIFACTS_URL", srv.URL)
	t.Setenv("ARTIFACTS_ACCOUNT", "local")
	t.Setenv("ARTIFACTS_API_TOKEN", "control-secret")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(context.Background(), []string{"doctor"}, stdout, stderr); code != 0 {
		t.Fatalf("doctor code=%d stderr=%s", code, stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"verify-core", "--evidence", evidence}, stdout, stderr); code != 0 {
		t.Fatalf("verify core code=%d stderr=%s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(evidence, "report.json")) //nolint:gosec // test-owned temporary directory
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Classification != classNone {
		t.Fatalf("classification %q", got.Classification)
	}
	for _, name := range []string{"report.json", "report.md", "git.log"} {
		if info, err := os.Stat(filepath.Join(evidence, name)); err != nil || info.Size() == 0 {
			t.Fatalf("evidence %s: %v size=%d", name, err, size(info))
		}
	}
	namespace, repo := "agent-"+got.RunID, "core-"+got.RunID
	response, _, err := h.request(context.Background(), http.MethodGet, repoPath(namespace, repo), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("repository final state %d, want 404", response.StatusCode)
	}
}

func size(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}
