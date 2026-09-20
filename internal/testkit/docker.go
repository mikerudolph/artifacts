package testkit

import (
	"os/exec"
	"testing"
)

func dockerAvailable() bool {
	path, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	return exec.Command(path, "info").Run() == nil //nolint:gosec
}

func DockerAvailable(tb testing.TB) {
	tb.Helper()
	if !dockerAvailable() {
		tb.Skip("docker not available")
	}
}
