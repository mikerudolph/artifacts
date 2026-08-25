package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// CredentialIssuer mints opaque short-lived repository credentials.
type CredentialIssuer interface {
	Issue(ctx context.Context, repo types.RepoID, scope types.Scope, ttlSeconds int) (types.CreateTokenResult, error)
}

type credentialIssuer struct {
	tokens meta.RepoTokens
	now    func() time.Time
}

// NewCredentialIssuer returns the built-in hashed opaque credential issuer.
func NewCredentialIssuer(tokens meta.RepoTokens, now func() time.Time) CredentialIssuer {
	if now == nil {
		now = time.Now
	}
	return &credentialIssuer{tokens: tokens, now: now}
}

func (i *credentialIssuer) Issue(ctx context.Context, repo types.RepoID, scope types.Scope, ttlSeconds int) (types.CreateTokenResult, error) {
	now := i.now()
	plain, hash, id, expires, err := MintRepo(scope, time.Duration(ttlSeconds)*time.Second, now)
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	_, err = i.tokens.Create(ctx, types.RepoToken{
		ID: id, RepoID: repo, Hash: hash, Scope: scope, State: types.TokenActive, CreatedAt: now, ExpiresAt: expires,
	})
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	return types.CreateTokenResult{ID: id, Plaintext: plain, Scope: scope, ExpiresAt: expires}, nil
}

// MintAPI creates a control-plane token and its durable hash.
func MintAPI() (string, string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain := "art_api_v1_" + hex.EncodeToString(b)
	return plain, HashAPI(plain), nil
}

// ControlAuthorizer validates tenant-bound control-plane credentials.
type ControlAuthorizer struct {
	tokens meta.APITokens
}

// NewControlAuthorizer constructs a tenant-bound control-plane authorizer.
func NewControlAuthorizer(tokens meta.APITokens) *ControlAuthorizer {
	return &ControlAuthorizer{tokens: tokens}
}

// Authorize verifies the hash and account binding.
func (a *ControlAuthorizer) Authorize(ctx context.Context, account types.AccountID, plaintext string) error {
	token, err := a.tokens.GetByHash(ctx, HashAPI(plaintext))
	if err != nil {
		return err
	}
	if token.AccountID != account {
		return ErrMismatch
	}
	return VerifyAPI(plaintext, token.Hash)
}
