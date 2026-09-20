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
