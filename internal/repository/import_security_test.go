package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestControlledImportArgumentsAndEnvironment(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/tmp/attacker-config")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9999")
	t.Setenv("https_proxy", "http://127.0.0.1:9998")
	args, err := importArgs(types.ImportSpec{
		URL: "https://git.example/repo.git", PinnedAddress: "93.184.216.34", Branch: "main", Depth: 1,
	}, "/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{
		"http.curloptResolve=git.example:443:93.184.216.34", "http.followRedirects=false",
		"http.proxy=", "credential.helper=", "protocol.file.allow=never", "--single-branch",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing secure option %q in %q", required, joined)
		}
	}
	env := strings.Join(gitEnvironment(nil), "\n")
	if strings.Contains(env, "/tmp/attacker-config") || strings.Contains(env, "GIT_CONFIG_COUNT=1") {
		t.Fatalf("ambient git configuration survived: %s", env)
	}
	if strings.Contains(env, "HTTP_PROXY=") || strings.Contains(env, "https_proxy=") {
		t.Fatalf("ambient proxy survived: %s", env)
	}
	if !strings.Contains(env, "GIT_CONFIG_GLOBAL=/dev/null") || !strings.Contains(env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatal("secure git environment missing")
	}
}

func TestImportObjectCountingStopsAtLimit(t *testing.T) {
	testkit.GitAvailable(t)
	repo := testkit.TempRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "one"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, repo, "add", "one")
	testkit.RunGit(t, repo, "-c", "commit.gpgsign=false", "commit", "-m", "one")
	if err := validateImportUsage(context.Background(), filepath.Join(repo, ".git"), types.ImportSpec{
		MaxBytes: 1 << 20, MaxObjects: 1,
	}); !errors.Is(err, errImportLimit) {
		t.Fatalf("object limit: %v", err)
	}
}

func TestImportHardFileQuota(t *testing.T) {
	bin := installFakeGit(t, "#!/bin/sh\nmkdir -p \"$1\"\nexec dd if=/dev/zero of=\"$1/large\" bs=1M count=64\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := filepath.Join(t.TempDir(), "clone")
	err := runBoundedImport(context.Background(), root, types.ImportSpec{
		Timeout: time.Second, MaxBytes: 1024,
	}, []string{root})
	if !errors.Is(err, errImportLimit) {
		t.Fatalf("hard quota: %v", err)
	}
	info, statErr := os.Stat(filepath.Join(root, "large"))
	if statErr == nil && info.Size() > 1024 {
		t.Fatalf("quota file grew to %d bytes", info.Size())
	}
}

func TestImportFileLimitBlocks(t *testing.T) {
	tests := []struct {
		name     string
		maxBytes int64
		goos     string
		want     int64
	}{
		{name: "linux exact", maxBytes: 1024, goos: "linux", want: 2},
		{name: "linux rounds up", maxBytes: 1025, goos: "linux", want: 3},
		{name: "darwin exact", maxBytes: 1024, goos: "darwin", want: 1},
		{name: "darwin rounds up", maxBytes: 1025, goos: "darwin", want: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := importFileLimitBlocks(test.maxBytes, test.goos); got != test.want {
				t.Fatalf("blocks = %d, want %d", got, test.want)
			}
		})
	}
}

func TestControlledImportLimitsAndTimeout(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "oversized"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateImportUsage(context.Background(), root, types.ImportSpec{MaxBytes: 4}); !errors.Is(err, errImportLimit) {
		t.Fatalf("size limit %v", err)
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "git")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o700); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := runBoundedImport(context.Background(), filepath.Join(root, "clone"), types.ImportSpec{
		Timeout: 25 * time.Millisecond, MaxBytes: 1024, MaxObjects: 10,
	}, []string{"clone"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error %v", err)
	}
}

func TestControlledImportRejectsUnpinnedPolicy(t *testing.T) {
	manager := &Manager{}
	_, err := manager.ImportControlled(context.Background(), types.Repo{}, types.ImportSpec{})
	if err == nil {
		t.Fatal("accepted unpinned import")
	}
	_, err = manager.ImportControlled(context.Background(), types.Repo{}, types.ImportSpec{
		PinnedAddress: "93.184.216.34", MaxBytes: 1, MaxObjects: 1, Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("accepted repository without identity")
	}
	for _, address := range []string{"not-an-ip", "100.64.0.1", "198.18.0.1", "224.0.0.1", "2001:db8::1", "ff02::1"} {
		if _, err := importArgs(types.ImportSpec{URL: "https://git.example/x", PinnedAddress: address}, t.TempDir()); err == nil {
			t.Fatalf("accepted invalid pin %s", address)
		}
	}
}

func TestImportProcessBoundsAndErrors(t *testing.T) {
	buffer := &cappedBuffer{limit: 3}
	if n, err := buffer.Write([]byte("abcdef")); err != nil || n != 6 || buffer.String() != "abc" {
		t.Fatalf("capped write: %d %q %v", n, buffer.String(), err)
	}
	if n, err := buffer.Write([]byte("z")); err != nil || n != 1 || buffer.String() != "abc" {
		t.Fatalf("full capped write: %d %q %v", n, buffer.String(), err)
	}
	if size, err := importUsage(filepath.Join(t.TempDir(), "missing")); err != nil || size != 0 {
		t.Fatalf("missing usage: %d %v", size, err)
	}

	t.Run("clone failure", func(t *testing.T) {
		bin := installFakeGit(t, "#!/bin/sh\necho controlled failure >&2\nexit 7\n")
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		err := runBoundedImport(context.Background(), t.TempDir(), types.ImportSpec{Timeout: time.Second}, []string{"clone"})
		if err == nil || !strings.Contains(err.Error(), "controlled failure") {
			t.Fatalf("clone failure: %v", err)
		}
	})
	t.Run("live disk limit", func(t *testing.T) {
		bin := installFakeGit(t, "#!/bin/sh\nmkdir -p \"$1\"\nprintf 12345 > \"$1/large\"\nexec sleep 5\n")
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		root := filepath.Join(t.TempDir(), "clone")
		err := runBoundedImport(context.Background(), root, types.ImportSpec{
			Timeout: time.Second, MaxBytes: 4,
		}, []string{root})
		if !errors.Is(err, errImportLimit) {
			t.Fatalf("disk limit: %v", err)
		}
	})
}

func installFakeGit(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	script := filepath.Join(bin, "git")
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o700); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	return bin
}
