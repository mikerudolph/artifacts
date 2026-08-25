package auth

import (
	"context"
	"errors"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// RepoAuthorizer validates opaque repository credentials against durable state.
type RepoAuthorizer struct {
	tokens meta.RepoTokens
	now    func() time.Time
}

// NewRepoAuthorizer constructs a repository-scoped authorizer.
func NewRepoAuthorizer(tokens meta.RepoTokens, now func() time.Time) *RepoAuthorizer {
	if now == nil {
		now = time.Now
	}
	return &RepoAuthorizer{tokens: tokens, now: now}
}

// Authorize validates tenant-resolved repository, scope, state, and stored expiry.
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
		return errors.New("credential is read only")
	}
	return nil
}
