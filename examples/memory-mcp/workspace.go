package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type workspace struct {
	dir        string
	credential string
	mu         sync.Mutex
}

func cloneWorkspace(ctx context.Context, remote, credential string) (*workspace, error) {
	parent, err := os.MkdirTemp("", "artifacts-memory-*")
	if err != nil {
		return nil, errors.New("create disposable clone")
	}
	w := &workspace{dir: filepath.Join(parent, "brain"), credential: credential}
	if err := w.git(ctx, parent, true, "clone", remote, w.dir); err != nil {
		_ = os.RemoveAll(parent)
		return nil, err
	}
	if err := w.git(ctx, w.dir, false, "config", "user.name", "Artifacts Memory MCP"); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.git(ctx, w.dir, false, "config", "user.email", "memory@artifacts.local"); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.git(ctx, w.dir, false, "config", "commit.gpgsign", "false"); err != nil {
		_ = w.Close()
		return nil, err
	}
	return w, nil
}

func (w *workspace) Close() error {
	return os.RemoveAll(filepath.Dir(w.dir))
}

func (w *workspace) read(ctx context.Context, visit func(string) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.refresh(ctx); err != nil {
		return err
	}
	return visit(w.dir)
}

func (w *workspace) mutate(ctx context.Context, message string, apply func(string) error) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.refresh(ctx); err != nil {
		return "", err
	}
	old, _ := w.gitOutput(ctx, w.dir, false, "rev-parse", "--verify", "HEAD")
	old = strings.TrimSpace(old)
	if err := apply(w.dir); err != nil {
		w.restore(ctx)
		return "", err
	}
	if err := w.git(ctx, w.dir, false, "add", "-A"); err != nil {
		w.restore(ctx)
		return "", err
	}
	if err := w.git(ctx, w.dir, false, "commit", "-m", message); err != nil {
		w.restore(ctx)
		return "", err
	}
	if err := w.push(ctx); err != nil {
		w.discard(ctx, old)
		return "", err
	}
	sha, err := w.gitOutput(ctx, w.dir, false, "rev-parse", "HEAD")
	return strings.TrimSpace(sha), err
}

func (w *workspace) refresh(ctx context.Context) error {
	if err := w.git(ctx, w.dir, true, "fetch", "origin"); err != nil {
		return err
	}
	if w.git(ctx, w.dir, false, "rev-parse", "--verify", "refs/remotes/origin/main") != nil {
		return nil
	}
	if w.git(ctx, w.dir, false, "rev-parse", "--verify", "HEAD") != nil {
		return w.git(ctx, w.dir, false, "checkout", "-B", "main", "origin/main")
	}
	return w.git(ctx, w.dir, false, "merge", "--ff-only", "origin/main")
}

func (w *workspace) push(ctx context.Context) error {
	if err := w.git(ctx, w.dir, true, "push", "origin", "HEAD:main"); err == nil {
		return nil
	}
	for range 3 {
		if err := w.git(ctx, w.dir, true, "fetch", "origin", "main"); err != nil {
			return err
		}
		if err := w.git(ctx, w.dir, false, "rebase", "origin/main"); err != nil {
			_ = w.git(ctx, w.dir, false, "rebase", "--abort")
			return errors.New("conflicting memory publication")
		}
		if err := w.git(ctx, w.dir, true, "push", "origin", "HEAD:main"); err == nil {
			return nil
		}
	}
	return errors.New("memory publication retry exhausted")
}

func (w *workspace) restore(ctx context.Context) {
	_ = w.git(ctx, w.dir, false, "reset", "--hard", "HEAD")
	_ = w.git(ctx, w.dir, false, "clean", "-fd")
}

func (w *workspace) discard(ctx context.Context, old string) {
	if old != "" {
		_ = w.git(ctx, w.dir, false, "reset", "--hard", old)
	} else {
		branch, _ := w.gitOutput(ctx, w.dir, false, "symbolic-ref", "--short", "HEAD")
		_ = w.git(ctx, w.dir, false, "update-ref", "-d", "refs/heads/"+strings.TrimSpace(branch))
	}
	_ = w.git(ctx, w.dir, false, "clean", "-fd")
}

func (w *workspace) git(ctx context.Context, dir string, auth bool, args ...string) error {
	_, err := w.gitOutput(ctx, dir, auth, args...)
	return err
}

func (w *workspace) gitOutput(ctx context.Context, dir string, auth bool, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if auth && w.credential != "" {
		secret, _, _ := strings.Cut(w.credential, "?")
		header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("agent:"+secret))
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=protocol.version", "GIT_CONFIG_VALUE_0=1",
			"GIT_CONFIG_KEY_1=http.extraHeader", "GIT_CONFIG_VALUE_1="+header)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		op := "unknown"
		if len(args) > 0 {
			op = args[0]
		}
		return "", fmt.Errorf("git %s failed", op)
	}
	return string(output), nil
}
