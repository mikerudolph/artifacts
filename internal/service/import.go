package service

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Import clones a remote into a new repo via jobs.Runner.
func (s *Services) Import(ctx context.Context, objects object.Store, account types.AccountID, ns string, name types.RepoName, in types.ImportRepoInput) (types.CreateRepoResult, error) {
	return jobs.New(s.meta, objects, s.publicURL).Import(ctx, account, ns, name, in)
}
