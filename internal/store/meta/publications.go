package meta

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/types"
)

type PublicationReader interface {
	PublicationEvents(context.Context, types.RepoID, int64, int) ([]types.PublicationEvent, error)
}

type CompactionSource interface {
	CompactionCandidates(context.Context, int64, int) ([]types.RepoID, error)
}

type SnapshotReader interface {
	RepositorySnapshot(context.Context, types.RepoID) (types.Repo, []types.Ref, error)
}

type CompactionCoordinator interface {
	RunCompaction(context.Context, types.RepoID, func(V2Store) error) error
}
