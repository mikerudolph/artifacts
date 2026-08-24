package service

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Fork copies a repo via jobs.Runner.
func (s *Services) Fork(ctx context.Context, objects object.Store, account types.AccountID, srcNS, srcName string, dst types.ForkRepoInput) (types.CreateRepoResult, error) {
	return jobs.New(s.meta, objects, s.publicURL).Fork(ctx, account, srcNS, srcName, dst)
}
