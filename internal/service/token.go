package service

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/auth"
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
	if err := s.meta.Accounts().Ensure(ctx, account); err != nil {
		return err
	}
	_, err := types.ParseNamespaceName(ns)
	if err != nil {
		return err
	}
	return s.meta.RepoTokens().Revoke(ctx, id)
}

func (s *Services) mintAndStore(ctx context.Context, repoID types.RepoID, scope types.Scope, ttlSec int) (types.CreateTokenResult, error) {
	now := s.now()
	plain, hash, id, exp, err := auth.MintRepo(scope, time.Duration(ttlSec)*time.Second, now)
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	_, err = s.meta.RepoTokens().Create(ctx, types.RepoToken{
		ID:        id,
		RepoID:    repoID,
		Hash:      hash,
		Scope:     scope,
		State:     types.TokenActive,
		CreatedAt: now,
		ExpiresAt: exp,
	})
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	return types.CreateTokenResult{ID: id, Plaintext: plain, Scope: scope, ExpiresAt: exp}, nil
}
