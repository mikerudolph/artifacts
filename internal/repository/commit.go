package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

const (
	maxCommitFiles = 100
	maxCommitBytes = 1 << 20
)

// Commit validates and publishes a bounded REST commit.
func (m *Manager) Commit(ctx context.Context, repo types.Repo, input types.CommitInput) (types.CommitResult, error) {
	branch, err := validateCommit(repo, input)
	if err != nil {
		return types.CommitResult{}, err
	}
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return types.CommitResult{}, err
	}
	defer unlock()
	if err := m.ensure(ctx, repo, path); err != nil {
		return types.CommitResult{}, err
	}
	work, err := os.MkdirTemp(m.root, ".rest-commit-*")
	if err != nil {
		return types.CommitResult{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	ref := "refs/heads/" + branch
	old := currentRef(ctx, path, ref)
	sha, err := writeCommit(ctx, path, work, old, input)
	if err != nil {
		return types.CommitResult{}, err
	}
	args := []string{"--git-dir=" + path, "update-ref", ref, sha}
	if old != "" {
		args = append(args, old)
	}
	if _, err := runGit(ctx, nil, args...); err != nil {
		return types.CommitResult{}, err
	}
	pack, err := m.packAndUpload(ctx, repo, path)
	if err != nil {
		_ = os.RemoveAll(path)
		return types.CommitResult{}, err
	}
	sequence, err := m.meta.WAL().Publish(ctx, types.Publication{
		Pack: pack, Updates: []types.RefUpdate{{Name: ref, OldSHA: old, NewSHA: sha}},
		ExpectedSequence: repo.WALSequence, AllowReadOnly: repo.WALSequence == 0,
	})
	if err != nil {
		_ = os.RemoveAll(path)
		return types.CommitResult{}, err
	}
	if err := writeCacheState(path, sequence, repo.DefaultBranch); err != nil {
		_ = os.RemoveAll(path)
	}
	return types.CommitResult{SHA: sha, Sequence: sequence}, nil
}

func validateCommit(repo types.Repo, input types.CommitInput) (string, error) {
	branch := input.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if _, err := types.ParseBranchName(branch); err != nil {
		return "", err
	}
	if len(input.Files) == 0 || len(input.Files) > maxCommitFiles {
		return "", errors.New("commit must contain 1 to 100 files")
	}
	total := 0
	seen := map[string]struct{}{}
	for _, file := range input.Files {
		clean := filepath.ToSlash(filepath.Clean(file.Path))
		if clean != file.Path || clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/.git/") || strings.HasPrefix(clean, ".git/") {
			return "", errors.New("invalid commit path")
		}
		if _, ok := seen[clean]; ok {
			return "", errors.New("duplicate commit path")
		}
		seen[clean] = struct{}{}
		total += len(file.Content)
	}
	if total > maxCommitBytes {
		return "", errors.New("commit content exceeds limit")
	}
	return branch, nil
}

func writeCommit(ctx context.Context, gitDir, work, parent string, input types.CommitInput) (string, error) {
	index := filepath.Join(gitDir, ".artifacts-index")
	defer func() { _ = os.Remove(index) }()
	env := []string{"GIT_INDEX_FILE=" + index}
	readArgs := []string{"--git-dir=" + gitDir, "--work-tree=" + work, "read-tree", "--empty"}
	if parent != "" {
		readArgs[len(readArgs)-1] = parent
	}
	if _, err := runGitEnv(ctx, nil, env, "", readArgs...); err != nil {
		return "", err
	}
	for _, file := range input.Files {
		dest := filepath.Join(work, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return "", err
		}
		if err := os.WriteFile(dest, []byte(file.Content), 0o600); err != nil {
			return "", err
		}
	}
	if _, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "--work-tree="+work, "add", "-A"); err != nil {
		return "", err
	}
	tree, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "write-tree")
	if err != nil {
		return "", err
	}
	return createCommit(ctx, gitDir, strings.TrimSpace(string(tree)), parent, input)
}

func createCommit(ctx context.Context, gitDir, tree, parent string, input types.CommitInput) (string, error) {
	message := input.Message
	if message == "" {
		message = "Initial artifacts"
	}
	when := input.Author.When
	if when.IsZero() {
		when = time.Now().UTC()
	}
	name, email := input.Author.Name, input.Author.Email
	if name == "" {
		name = "Artifacts Agent"
	}
	if email == "" {
		email = "agent@artifacts.local"
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_AUTHOR_DATE=" + when.Format(time.RFC3339),
		"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email, "GIT_COMMITTER_DATE=" + when.Format(time.RFC3339),
	}
	args := []string{"--git-dir=" + gitDir, "commit-tree", tree, "-m", message}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	b, err := runGitEnv(ctx, nil, env, "", args...)
	return strings.TrimSpace(string(b)), err
}

func currentRef(ctx context.Context, path, ref string) string {
	b, err := runGit(ctx, nil, "--git-dir="+path, "rev-parse", "--verify", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Refs lists published refs.
func (m *Manager) Refs(ctx context.Context, repo types.Repo) ([]types.Ref, error) {
	return m.meta.Refs().List(ctx, repo.ID)
}

// WAL lists published immutable packs.
func (m *Manager) WAL(ctx context.Context, repo types.Repo) ([]types.PackWAL, error) {
	return m.meta.WAL().List(ctx, repo.ID, 0, -1)
}

// Compact publishes the current full pack as a checkpoint.
func (m *Manager) Compact(ctx context.Context, repo types.Repo) error {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.ensure(ctx, repo, path); err != nil {
		return err
	}
	pack, err := m.packAndUpload(ctx, repo, path)
	if err != nil {
		return err
	}
	return m.meta.Checkpoints().Put(ctx, types.Checkpoint{
		RepoID: repo.ID, Sequence: repo.WALSequence, PackKey: pack.PackKey,
		IndexKey: pack.IndexKey, Checksum: pack.Checksum,
	})
}
