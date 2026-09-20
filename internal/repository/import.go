package repository

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mikerudolph/artifacts/internal/netpolicy"
	"github.com/mikerudolph/artifacts/internal/types"
)

var errImportLimit = errors.New("import limit exceeded")

func (m *Manager) Import(ctx context.Context, repo types.Repo, remote, branch string, depth int) error {
	_, err := m.importRepo(ctx, repo, types.ImportSpec{URL: remote, Branch: branch, Depth: depth})
	return err
}

func (m *Manager) ImportControlled(ctx context.Context, repo types.Repo, spec types.ImportSpec) (string, error) {
	if spec.PinnedAddress == "" || spec.MaxBytes <= 0 || spec.MaxObjects <= 0 || spec.Timeout <= 0 {
		return "", errors.New("controlled import policy is required")
	}
	return m.importRepo(ctx, repo, spec)
}

func (m *Manager) importRepo(ctx context.Context, repo types.Repo, spec types.ImportSpec) (string, error) {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := os.RemoveAll(path); err != nil {
		return "", err
	}
	args, err := importArgs(spec, path)
	if err != nil {
		return "", err
	}
	if err := runBoundedImport(ctx, path, spec, args); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	branch, err := importedDefaultBranch(ctx, path, spec.Branch)
	if err != nil {
		return "", err
	}
	repo.DefaultBranch = branch
	return branch, m.publishImport(ctx, repo, path)
}

func importArgs(spec types.ImportSpec, path string) ([]string, error) {
	args := []string{"clone", "--bare"}
	if spec.PinnedAddress != "" {
		u, err := url.Parse(spec.URL)
		ip := net.ParseIP(spec.PinnedAddress)
		if err != nil || u.Scheme != "https" || !netpolicy.IsGloballyRoutable(ip) {
			return nil, errors.New("invalid controlled import target")
		}
		port := u.Port()
		if port == "" {
			port = "443"
		}
		address := spec.PinnedAddress
		if strings.Contains(address, ":") {
			address = "[" + address + "]"
		}
		resolve := u.Hostname() + ":" + port + ":" + address
		args = append(args, "-c", "http.curloptResolve="+resolve, "-c", "http.followRedirects=false",
			"-c", "http.proxy=", "-c", "credential.helper=", "-c", "protocol.file.allow=never")
	}
	if spec.Branch != "" {
		args = append(args, "--branch", spec.Branch, "--single-branch")
	}
	if spec.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(spec.Depth))
	}
	return append(args, "--", spec.URL, path), nil
}

func runBoundedImport(parent context.Context, path string, spec types.ImportSpec, args []string) error {
	ctx := parent
	cancel := func() {}
	if spec.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, spec.Timeout)
	}
	defer cancel()
	cmd := boundedGitCommand(ctx, spec.MaxBytes, args)
	cmd.Env = gitEnvironment(nil)
	stderr := &cappedBuffer{limit: 64 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				if importFileLimit(err) {
					return errImportLimit
				}
				return fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(stderr.String()))
			}
			return validateImportUsage(ctx, path, spec)
		case <-ctx.Done():
			<-done
			return ctx.Err()
		case <-ticker.C:
			size, err := importUsage(path)
			if err == nil && spec.MaxBytes > 0 && size > spec.MaxBytes {
				cancel()
				<-done
				return errImportLimit
			}
		}
	}
}

func boundedGitCommand(ctx context.Context, maxBytes int64, args []string) *exec.Cmd {
	if maxBytes <= 0 {
		return exec.CommandContext(ctx, "git", args...)
	}
	blocks := importFileLimitBlocks(maxBytes, runtime.GOOS)
	shell := []string{"-c", `ulimit -f "$1" || exit 125; shift; exec "$@"`, "artifacts-import",
		strconv.FormatInt(blocks, 10), "git"}
	return exec.CommandContext(ctx, "/bin/sh", append(shell, args...)...) //nolint:gosec
}

func importFileLimitBlocks(maxBytes int64, goos string) int64 {
	blockBytes := int64(512)
	if goos == "darwin" {
		blockBytes = 1024
	}
	return (maxBytes + blockBytes - 1) / blockBytes
}

func importFileLimit(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGXFSZ
}

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		return original, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, err := b.Buffer.Write(p)
	return original, err
}

func validateImportUsage(ctx context.Context, path string, spec types.ImportSpec) error {
	size, err := importUsage(path)
	if err != nil {
		return err
	}
	if spec.MaxBytes > 0 && size > spec.MaxBytes {
		return errImportLimit
	}
	return countImportObjects(ctx, path, spec.MaxObjects)
}

func countImportObjects(ctx context.Context, path string, limit int) error {
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+path, "rev-list", "--objects", "--all") //nolint:gosec
	cmd.Env = gitEnvironment(nil)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &cappedBuffer{limit: 64 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	count := 0
	for scanner.Scan() {
		count++
		if limit > 0 && count > limit {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return errImportLimit
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git rev-list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func importedDefaultBranch(ctx context.Context, path, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	out, err := runGit(ctx, nil, "--git-dir="+path, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	branch, err := types.ParseBranchName(strings.TrimSpace(string(out)))
	return string(branch), err
}

func importUsage(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return size, err
}

func (m *Manager) publishImport(ctx context.Context, repo types.Repo, path string) error {
	after, err := listRefs(ctx, path)
	if err != nil {
		return err
	}
	pack, err := m.packAndUpload(ctx, repo, path)
	if err != nil {
		return err
	}
	sequence, err := m.meta.WAL().Publish(ctx, types.Publication{
		Pack: pack, Updates: refDiff(map[string]string{}, after), ExpectedSequence: 0,
		AllowReadOnly: true, AllowNonReady: true,
	})
	if err != nil {
		_ = os.RemoveAll(path)
		return err
	}
	return writeCacheState(path, sequence, repo.DefaultBranch)
}
