package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

// RPC streams a read-only stateless Git service against a synchronized cache.
func (m *Manager) RPC(ctx context.Context, repo types.Repo, service string, input io.Reader, output io.Writer, protocol string) error {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.ensure(ctx, repo, path); err != nil {
		return err
	}
	if service != "upload-pack" && (service != "receive-pack" || input != nil) {
		return fmt.Errorf("unsupported git service")
	}
	args := []string{service, "--stateless-rpc"}
	if input == nil {
		args = append(args, "--advertise-refs")
	}
	args = append(args, path)
	return runGitStream(ctx, input, output, protocol, args...)
}

func runGitStream(ctx context.Context, input io.Reader, output io.Writer, protocol string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdin, cmd.Stdout = input, output
	cmd.Env = gitEnvironment(nil)
	if protocol != "" {
		cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+strings.TrimSpace(protocol))
	}
	stderr := &cappedBuffer{limit: 64 << 10}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func runGit(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
	return runGitProtocol(ctx, input, "", args...)
}

func runGitProtocol(ctx context.Context, input io.Reader, protocol string, args ...string) ([]byte, error) {
	return runGitEnv(ctx, input, nil, protocol, args...)
}

func runGitEnv(ctx context.Context, input io.Reader, extra []string, protocol string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdin = input
	cmd.Env = gitEnvironment(extra)
	if protocol != "" {
		cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+strings.TrimSpace(protocol))
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func gitEnvironment(extra []string) []string {
	blocked := []string{"GIT_CONFIG_", "GIT_ASKPASS=", "SSH_ASKPASS=", "GCM_INTERACTIVE="}
	proxy := map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true,
		"http_proxy": true, "https_proxy": true, "all_proxy": true}
	env := make([]string, 0, len(os.Environ())+len(extra)+6)
	for _, item := range os.Environ() {
		keep := true
		key, _, _ := strings.Cut(item, "=")
		if proxy[key] {
			continue
		}
		for _, prefix := range blocked {
			if strings.HasPrefix(item, prefix) {
				keep = false
				break
			}
		}
		if keep {
			env = append(env, item)
		}
	}
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/false", "SSH_ASKPASS=/bin/false", "GCM_INTERACTIVE=never")
	return append(env, extra...)
}
