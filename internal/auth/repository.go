package auth

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type RepoAuthorizer struct {
	tokens meta.RepoTokens
	now    func() time.Time
}

func NewRepoAuthorizer(tokens meta.RepoTokens, now func() time.Time) *RepoAuthorizer {
	if now == nil {
		now = time.Now
	}
	return &RepoAuthorizer{tokens: tokens, now: now}
}

func (a *RepoAuthorizer) Authorize(ctx context.Context, repo types.Repo, plaintext string, write bool) error {
	secret, embeddedExpiry, err := ParseRepo(plaintext)
	if err != nil {
		return err
	}
	token, err := a.tokens.GetByHash(ctx, HashRepo(secret))
	if err != nil {
		return err
	}
	now := a.now()
	if token.RepoID != repo.ID || token.State != types.TokenActive {
		return ErrMismatch
	}
	if !token.ExpiresAt.After(now) || (!embeddedExpiry.IsZero() && !embeddedExpiry.After(now)) {
		return ErrExpired
	}
	if write && token.Scope != types.ScopeWrite {
		return types.ErrForbidden
	}
	return nil
}
