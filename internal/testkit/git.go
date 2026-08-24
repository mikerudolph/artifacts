package testkit

import (
	"os/exec"
	"testing"
)

// GitAvailable skips the test when git is not on PATH.
func GitAvailable(tb testing.TB) {
	tb.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		tb.Skip("git not on PATH")
	}
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// RunGit runs git in dir and fails the test on a non-zero exit.
func RunGit(tb testing.TB, dir string, args ...string) string {
	tb.Helper()
	GitAvailable(tb)
	out, err := gitOutput(dir, args...)
	if err != nil {
		tb.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

// TempRepo creates a git repo with user.name and user.email set.
func TempRepo(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	RunGit(tb, dir, "init", "-b", "main")
	RunGit(tb, dir, "config", "user.name", "artifacts")
	RunGit(tb, dir, "config", "user.email", "artifacts@test")
	return dir
}
