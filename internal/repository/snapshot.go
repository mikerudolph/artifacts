package repository

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (m *Manager) snapshot(ctx context.Context, repo types.Repo) (types.Repo, []types.Ref, error) {
	if reader, ok := m.meta.(meta.SnapshotReader); ok {
		return reader.RepositorySnapshot(ctx, repo.ID)
	}
	refs, err := m.meta.Refs().List(ctx, repo.ID)
	return repo, refs, err
}

func (m *Manager) prepare(ctx context.Context, repo types.Repo, path string) (types.Repo, error) {
	current, refs, err := m.snapshot(ctx, repo)
	if err != nil {
		return types.Repo{}, err
	}
	if err := m.ensureSnapshot(ctx, current, refs, path); err != nil {
		return types.Repo{}, err
	}
	if current.StorageVersion == 1 {
		return m.currentRepo(ctx, current)
	}
	return current, nil
}

func (m *Manager) commitSnapshot(ctx context.Context, repo types.Repo, input types.CommitInput) (types.Repo, []types.Ref, error) {
	current, refs, err := m.snapshot(ctx, repo)
	if err != nil {
		return types.Repo{}, nil, err
	}
	if current.ReadOnly && (current.WALSequence != 0 || input.RepositoryCredential) {
		return types.Repo{}, nil, types.ErrForbidden
	}
	return current, refs, nil
}
