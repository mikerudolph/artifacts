package testkit

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTempRepoAndGit(t *testing.T) {
	t.Parallel()
	dir := TempRepo(t)
	out := RunGit(t, dir, "status")
	if dir == "" {
		t.Fatal("empty dir")
	}
	if !strings.Contains(out, "No commits yet") && !strings.Contains(out, "nothing to commit") && out == "" {
		t.Fatalf("status %q", out)
	}
	GitAvailable(t)
}

func TestPostgres(t *testing.T) {
	t.Parallel()
	dsn := Postgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestMinIO(t *testing.T) {
	t.Parallel()
	endpoint, access, secret, bucket := MinIO(t)
	if endpoint == "" || access == "" || secret == "" || bucket == "" {
		t.Fatalf("%q %q %q %q", endpoint, access, secret, bucket)
	}
	resp, err := http.Get("http://" + strings.TrimPrefix(endpoint, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestDockerLookPathMiss(t *testing.T) {
	t.Setenv("PATH", "")
	if dockerAvailable() {
		t.Fatal("expected docker unavailable")
	}
}

func TestGitOutputError(t *testing.T) {
	t.Parallel()
	if _, err := gitOutput(t.TempDir(), "definitely-not-a-git-command"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDockerAvailableSkipPath(t *testing.T) {
	t.Parallel()
	if dockerAvailable() {
		DockerAvailable(t)
		return
	}
	DockerAvailable(t)
}
