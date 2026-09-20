package repository

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

const (
	maxCommitFiles = 100
	maxCommitBytes = 1 << 20
)

func (m *Manager) Commit(ctx context.Context, repo types.Repo, input types.CommitInput) (types.CommitResult, error) {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return types.CommitResult{}, err
	}
	defer unlock()
	if input.IdempotencyKey != "" {
		result, err := meta.Idempotent(ctx, m.meta, "commit/"+string(repo.AccountID)+"/"+string(repo.ID), input.IdempotencyKey, input, func(tx meta.Store) (types.CommitResult, error) {
			inner := &Manager{meta: tx.(meta.V2Store), objects: m.objects, root: m.root}
			return inner.commitLocked(ctx, repo, input, path)
		})
		if err != nil {
			_ = os.RemoveAll(path)
		}
		return result, err
	}
	return m.commitLocked(ctx, repo, input, path)
}

func (m *Manager) commitLocked(ctx context.Context, repo types.Repo, input types.CommitInput, path string) (types.CommitResult, error) {
	repo, refs, err := m.commitSnapshot(ctx, repo, input)
	if err != nil {
		return types.CommitResult{}, err
	}
	branch, err := validateCommit(repo, input)
	if err != nil {
		return types.CommitResult{}, err
	}
	if err := m.ensureSnapshot(ctx, repo, refs, path); err != nil {
		return types.CommitResult{}, err
	}
	work, err := os.MkdirTemp(m.root, ".rest-commit-*")
	if err != nil {
		return types.CommitResult{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	before, err := listRefs(ctx, path)
	if err != nil {
		return types.CommitResult{}, err
	}
	ref := "refs/heads/" + branch
	old := currentRef(ctx, path, ref)
	parent, err := commitParent(ctx, repo, input, path, old)
	if err != nil {
		return types.CommitResult{}, err
	}
	sha, err := writeCommit(ctx, path, work, parent, input)
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
	pack, err := m.incrementalPack(ctx, repo, path, before, map[string]string{ref: sha})
	if err != nil {
		_ = os.RemoveAll(path)
		return types.CommitResult{}, err
	}
	sequence, err := m.meta.WAL().Publish(ctx, types.Publication{
		Pack: pack, Updates: []types.RefUpdate{{Name: ref, OldSHA: old, NewSHA: sha}},
		ExpectedSequence: repo.WALSequence, AllowReadOnly: repo.WALSequence == 0 && !input.RepositoryCredential,
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

func commitParent(ctx context.Context, repo types.Repo, input types.CommitInput, path, old string) (string, error) {
	if input.ExpectedHead != nil && *input.ExpectedHead != old {
		return "", &types.HeadConflict{Current: old}
	}
	parent := old
	if input.Base != "" {
		if old != "" {
			return "", &types.InputError{Field: "/base", Message: "base is only valid when creating a branch"}
		}
		kind, err := runGit(ctx, nil, "--git-dir="+path, "cat-file", "-t", input.Base)
		if err != nil || strings.TrimSpace(string(kind)) != "commit" {
			return "", &types.InputError{Field: "/base", Message: "base commit does not exist in this repository"}
		}
		parent = input.Base
	} else if old == "" {
		parent = currentRef(ctx, path, "refs/heads/"+repo.DefaultBranch)
	}
	return parent, nil
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

func (m *Manager) Refs(ctx context.Context, repo types.Repo) ([]types.Ref, error) {
	return m.meta.Refs().List(ctx, repo.ID)
}

func (m *Manager) WAL(ctx context.Context, repo types.Repo) ([]types.PackWAL, error) {
	return m.meta.WAL().List(ctx, repo.ID, 0, -1)
}

func (m *Manager) Compact(ctx context.Context, repo types.Repo) error {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return err
	}
	defer unlock()
	if coordinator, ok := m.meta.(meta.CompactionCoordinator); ok {
		return coordinator.RunCompaction(ctx, repo.ID, func(tx meta.V2Store) error {
			inner := &Manager{meta: tx, objects: m.objects, root: m.root}
			return inner.compactLocked(ctx, repo, path)
		})
	}
	return m.compactLocked(ctx, repo, path)
}

func (m *Manager) compactLocked(ctx context.Context, repo types.Repo, path string) error {
	repo, err := m.prepare(ctx, repo, path)
	if err != nil {
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
