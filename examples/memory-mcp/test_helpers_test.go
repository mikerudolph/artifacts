package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testBrain(t *testing.T) (*brain, string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "brain.git")
	runTestGit(t, "", "init", "--bare", "--initial-branch=main", remote)
	workspace, err := cloneWorkspace(context.Background(), remote, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.Close() })
	b := newBrain(workspace)
	clock := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	b.now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}
	b.random = bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096))
	return b, remote
}

func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", args[0], err, output)
	}
	return string(output)
}
