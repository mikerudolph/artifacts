package service

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// CreateToken mints a repo-scoped git token.
func (s *Services) CreateToken(ctx context.Context, account types.AccountID, ns string, in types.CreateTokenInput) (types.CreateTokenResult, error) {
	scope, err := types.ParseScope(string(in.Scope))
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	ttl, err := types.ParseTTL(in.TTL)
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	repo, _, err := s.lookupRepo(ctx, account, ns, string(in.Repo))
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	return s.mintAndStore(ctx, repo.ID, scope, ttl)
}

// ListTokens lists tokens for a repo.
func (s *Services) ListTokens(ctx context.Context, account types.AccountID, ns, repo string, state types.TokenState, page types.OffsetPage) ([]types.RepoToken, types.OffsetResult, error) {
	r, _, err := s.lookupRepo(ctx, account, ns, repo)
	if err != nil {
		return nil, types.OffsetResult{}, err
	}
	return s.meta.RepoTokens().List(ctx, r.ID, state, page)
}

// RevokeToken revokes a repo token by id.
func (s *Services) RevokeToken(ctx context.Context, account types.AccountID, ns string, id types.TokenID) error {
	namespace, _, err := s.lookupNS(ctx, account, ns)
	if err != nil {
		return err
	}
	token, err := s.meta.RepoTokens().GetByID(ctx, id)
	if err != nil {
		return err
	}
	repo, err := s.meta.Repos().GetByID(ctx, token.RepoID)
	if err != nil || repo.NamespaceID != namespace.ID {
		return meta.ErrNotFound
	}
	return s.meta.RepoTokens().Revoke(ctx, id)
}

func (s *Services) mintAndStore(ctx context.Context, repoID types.RepoID, scope types.Scope, ttlSec int) (types.CreateTokenResult, error) {
	return s.issuer.Issue(ctx, repoID, scope, ttlSec)
}
