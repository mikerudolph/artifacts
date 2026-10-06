package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

func runBinary(ctx context.Context, h harness, record *recorder, state *driveState, total int64) error {
	fixture := binaryFixture{path: filepath.Join(state.work, "source.bin"), size: total - int64(len(binaryText)+len(binaryScript)), head: state.gitSHA}
	steps := []struct {
		name string
		run  func() (string, error)
	}{
		{"generate binary fixture", func() (string, error) { return generateBinary(&fixture) }},
		{"multipart commit", func() (string, error) {
			result, status, err := uploadBinary(ctx, h, state, fixture, "binary-"+record.report.RunID, fixture.manifest())
			if err != nil {
				return "", err
			}
			fixture.result = result
			return fmt.Sprintf("status=%d sha=%s bytes=%d sequence=%d", status, result.SHA, total, result.Sequence), record.assert("multipart commit accepted", status == 201 && result.SHA != "", fmt.Sprintf("HTTP %d; expected 201", status))
		}},
		{"binary REST readback", func() (string, error) {
			return binaryRead(ctx, h, record, state, fixture.result.SHA, fixture)
		}},
		{"Git verifies binary and mode", func() (string, error) { return binaryGit(ctx, record, state, fixture) }},
		{"Git changes binary", func() (string, error) { return binaryGitPush(ctx, record, state, &fixture) }},
		{"REST reads Git binary", func() (string, error) {
			read := fixture
			read.size++
			return binaryRead(ctx, h, record, state, state.gitSHA, read)
		}},
		{"multipart retry after Git advancement", func() (string, error) { return binaryRetry(ctx, h, record, state, fixture) }},
	}
	for _, step := range steps {
		if err := record.step(step.name, step.run); err != nil {
			return err
		}
	}
	return nil
}

func generateBinary(f *binaryFixture) (string, error) {
	file, err := os.Create(f.path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	_, err = io.CopyN(io.MultiWriter(file, hash), rand.Reader, f.size)
	f.digest = hex.EncodeToString(hash.Sum(nil))
	return fmt.Sprintf("bytes=%d sha256=%s", f.size, f.digest), err
}

func binaryRead(ctx context.Context, h harness, record *recorder, state *driveState, sha string, f binaryFixture) (string, error) {
	target := h.base + repoPath(state.namespace, state.repo) + "/file?ref=" + url.QueryEscape(sha) + "&path=binary.bin"
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+state.credential)
	response, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(response.Body, f.size+1))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha=%s bytes=%d", sha, n), record.assert("REST binary checksum matches "+sha, response.StatusCode == 200 && n == f.size && hex.EncodeToString(hash.Sum(nil)) == f.digest, "byte count and SHA-256 matched")
}

func binaryGit(ctx context.Context, record *recorder, state *driveState, f binaryFixture) (string, error) {
	for _, args := range [][]string{{"fetch", "origin"}, {"reset", "--hard", f.result.SHA}} {
		out, err := runGit(ctx, state.work, state.credential, args...)
		record.gitLog.WriteString("$ git " + args[0] + "\n" + out)
		if err != nil {
			return "", err
		}
	}
	digest, err := fileDigest(filepath.Join(state.work, "binary.bin"))
	if err != nil {
		return "", err
	}
	if err := record.assert("Git binary checksum matches REST", digest == f.digest, "SHA-256 matched"); err != nil {
		return "", err
	}
	mode, err := runGit(ctx, state.work, "", "ls-files", "--stage", "binary.sh")
	if err != nil {
		return "", err
	}
	return "binary.sh executable", record.assert("Git sees executable mode", strings.HasPrefix(mode, "100755 "), "100755")
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	return hex.EncodeToString(hash.Sum(nil)), err
}

func binaryGitPush(ctx context.Context, record *recorder, state *driveState, f *binaryFixture) (string, error) {
	path := filepath.Join(state.work, "binary.bin")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec
	if err != nil {
		return "", err
	}
	_, err = file.Write([]byte{0})
	_ = file.Close()
	if err != nil {
		return "", err
	}
	f.digest, err = fileDigest(path)
	if err != nil {
		return "", err
	}
	for _, args := range [][]string{{"add", "binary.bin"}, {"-c", "user.name=Verification Agent", "-c", "user.email=agent@example.local", "commit", "-m", "change binary"}, {"push", "origin", "HEAD:main"}} {
		out, err := runGit(ctx, state.work, state.credential, args...)
		record.gitLog.WriteString("$ git " + args[0] + "\n" + out)
		if err != nil {
			return "", err
		}
	}
	sha, err := runGit(ctx, state.work, "", "rev-parse", "HEAD")
	state.gitSHA = strings.TrimSpace(sha)
	return "sha=" + state.gitSHA, err
}

func binaryRetry(ctx context.Context, h harness, record *recorder, state *driveState, f binaryFixture) (string, error) {
	key := "binary-" + record.report.RunID
	result, status, err := uploadBinary(ctx, h, state, f, key, f.manifest())
	if err != nil {
		return "", err
	}
	if err := record.assert("binary retry returns original publication", status == 201 && result == f.result, "original SHA and sequence"); err != nil {
		return "", err
	}
	changed := f.manifest()
	changed.Message = "different operation"
	_, status, err = uploadBinary(ctx, h, state, f, key, changed)
	if err != nil {
		return "", err
	}
	if err := record.assert("changed keyed binary request conflicts", status == 409, "HTTP 409"); err != nil {
		return "", err
	}
	_, status, err = uploadBinary(ctx, h, state, f, key+"-stale", f.manifest())
	if err != nil {
		return "", err
	}
	if err := record.assert("stale binary expected head conflicts", status == 409, "HTTP 409"); err != nil {
		return "", err
	}
	wal, err := callAPI[[]types.PackWAL](ctx, h, "GET", repoPath(state.namespace, state.repo)+"/wal", nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("publications=%d", len(wal)), record.assert("binary retries publish no additional commits", len(wal) == 4, "exactly four REST/Git publications")
}
