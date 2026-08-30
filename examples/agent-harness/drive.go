package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

type driveState struct {
	namespace  string
	repo       string
	created    bool
	remote     string
	credential string
	work       string
	restSHA    string
	gitSHA     string
}

func verifyCore(ctx context.Context, h harness, evidence string) (report, error) {
	runID, err := uniqueID()
	if err != nil {
		return report{}, err
	}
	record := newRecorder(runID, h.cfg)
	state := driveState{namespace: "agent-" + runID, repo: "core-" + runID}
	state.work, err = os.MkdirTemp("", "artifacts-verify-*")
	if err != nil {
		return report{}, err
	}
	err = runCore(ctx, h, record, &state)
	cleanupErr := cleanup(ctx, h, record, state)
	if cleanupErr != nil {
		if err != nil {
			cleanupErr = classified(classCleanup, err.Error()+"; cleanup: "+cleanupErr.Error())
		}
		err = cleanupErr
	}
	if removeErr := os.RemoveAll(state.work); removeErr != nil {
		err = classified(classCleanup, "remove client scratch: "+removeErr.Error())
	}
	record.finish(err)
	if writeErr := record.write(evidence); writeErr != nil {
		err = classified(classEvidence, "write evidence: "+writeErr.Error())
		record.finish(err)
		return record.report, err
	}
	return record.report, err
}

func runCore(ctx context.Context, h harness, record *recorder, state *driveState) error {
	steps := []struct {
		name string
		run  func() (string, error)
	}{
		{"doctor", func() (string, error) { return doctor(ctx, h) }},
		{"REST create repository", func() (string, error) { return createRepository(ctx, h, record, state) }},
		{"REST commit", func() (string, error) { return restCommit(ctx, h, state) }},
		{"REST read", func() (string, error) { return restRead(ctx, h, record, state, "README.md", "# Agent verification\n") }},
		{"create Git credential", func() (string, error) { return createCredential(ctx, h, record, state) }},
		{"Git clone", func() (string, error) { return gitClone(ctx, record, state) }},
		{"Git verify REST commit", func() (string, error) { return gitVerify(ctx, record, state) }},
		{"Git commit and push", func() (string, error) { return gitPush(ctx, record, state) }},
		{"REST readback", func() (string, error) {
			return restRead(ctx, h, record, state, "result.txt", "agent finished successfully\n")
		}},
		{"refs and WAL visibility", func() (string, error) { return inspectPublication(ctx, h, record, state) }},
	}
	for _, step := range steps {
		if err := record.step(step.name, step.run); err != nil {
			return err
		}
	}
	return nil
}

func createRepository(ctx context.Context, h harness, record *recorder, state *driveState) (string, error) {
	created, err := callAPI[types.CreateRepoResult](ctx, h, http.MethodPost,
		"/namespaces/"+state.namespace+"/repos", types.CreateRepoInput{
			Name: types.RepoName(state.repo), Description: "agent verification",
		})
	if err != nil {
		return "", err
	}
	state.created = true
	state.remote = created.Remote
	record.addSecret(created.Token)
	return "repository=" + state.namespace + "/" + state.repo, record.assert("repository has Git remote", created.Remote != "", "remote present")
}

func restCommit(ctx context.Context, h harness, state *driveState) (string, error) {
	commit, err := callAPI[types.CommitResult](ctx, h, http.MethodPost, repoPath(state.namespace, state.repo)+"/commits",
		types.CommitInput{Message: "bootstrap verification", Files: []types.CommitFile{{Path: "README.md", Content: "# Agent verification\n"}}})
	if err != nil {
		return "", err
	}
	state.restSHA = commit.SHA
	return fmt.Sprintf("sha=%s sequence=%d", commit.SHA, commit.Sequence), nil
}

func restRead(ctx context.Context, h harness, record *recorder, state *driveState, path, want string) (string, error) {
	got, err := h.readFile(ctx, state.namespace, state.repo, path)
	if err != nil {
		return "", err
	}
	return "path=" + path, record.assert("REST content matches "+path, got == want, "content matched")
}

func createCredential(ctx context.Context, h harness, record *recorder, state *driveState) (string, error) {
	credential, err := callAPI[types.CreateTokenResult](ctx, h, http.MethodPost,
		"/namespaces/"+state.namespace+"/credentials", types.CreateTokenInput{
			Repo: types.RepoName(state.repo), Scope: types.ScopeWrite, TTL: 3600,
		})
	if err != nil {
		return "", err
	}
	state.credential = credential.Plaintext
	record.addSecret(credential.Plaintext)
	return "scope=" + string(credential.Scope), nil
}

func gitClone(ctx context.Context, record *recorder, state *driveState) (string, error) {
	out, err := runGit(ctx, "", state.credential, "clone", state.remote, state.work)
	record.gitLog.WriteString("$ git clone <remote> <scratch>\n" + out)
	return "cloned repository", err
}

func gitVerify(ctx context.Context, record *recorder, state *driveState) (string, error) {
	data, err := os.ReadFile(filepath.Join(state.work, "README.md"))
	if err != nil {
		return "", err
	}
	if err := record.assert("Git sees REST content", string(data) == "# Agent verification\n", "README matched"); err != nil {
		return "", err
	}
	out, err := runGit(ctx, state.work, "", "rev-parse", "HEAD")
	record.gitLog.WriteString("$ git rev-parse HEAD\n" + out)
	if err != nil {
		return "", err
	}
	return "head=" + strings.TrimSpace(out), record.assert("Git sees REST SHA", strings.TrimSpace(out) == state.restSHA, "SHA matched")
}

func gitPush(ctx context.Context, record *recorder, state *driveState) (string, error) {
	if err := os.WriteFile(filepath.Join(state.work, "result.txt"), []byte("agent finished successfully\n"), 0o600); err != nil {
		return "", err
	}
	commands := [][]string{{"add", "result.txt"}, {"-c", "user.name=Verification Agent", "-c", "user.email=agent@example.local", "commit", "-m", "add result"}}
	for _, args := range commands {
		out, err := runGit(ctx, state.work, "", args...)
		record.gitLog.WriteString("$ git " + args[0] + "\n" + out)
		if err != nil {
			return "", err
		}
	}
	out, err := runGit(ctx, state.work, state.credential, "push", "origin", "HEAD:main")
	record.gitLog.WriteString("$ git push origin HEAD:main\n" + out)
	if err != nil {
		return "", err
	}
	sha, err := runGit(ctx, state.work, "", "rev-parse", "HEAD")
	state.gitSHA = strings.TrimSpace(sha)
	return "sha=" + state.gitSHA, err
}

func inspectPublication(ctx context.Context, h harness, record *recorder, state *driveState) (string, error) {
	refs, err := callAPI[[]types.Ref](ctx, h, http.MethodGet, repoPath(state.namespace, state.repo)+"/refs", nil)
	if err != nil {
		return "", err
	}
	found := false
	for _, ref := range refs {
		found = found || ref.Name == "refs/heads/main" && ref.SHA == state.gitSHA
	}
	if err := record.assert("main ref exposes Git push", found, "refs/heads/main matched"); err != nil {
		return "", err
	}
	wal, err := callAPI[[]types.PackWAL](ctx, h, http.MethodGet, repoPath(state.namespace, state.repo)+"/wal", nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("refs=%d wal=%d", len(refs), len(wal)), record.assert("WAL exposes REST and Git publications", len(wal) >= 2, "at least two publications")
}

func cleanup(ctx context.Context, h harness, record *recorder, state driveState) error {
	if !state.created {
		return nil
	}
	err := record.step("delete repository", func() (string, error) {
		if err := deleteRepo(ctx, h, state.namespace, state.repo); err != nil {
			return "", err
		}
		exists, err := repoExists(ctx, h, state.namespace, state.repo)
		if err != nil {
			return "", err
		}
		return "final server state checked", record.assert("repository absent after cleanup", !exists, "GET returned not found")
	})
	if err != nil {
		return &classifiedError{class: classCleanup, err: err}
	}
	return nil
}

func uniqueID() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
