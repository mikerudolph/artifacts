package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type tokenMemory struct {
	repo map[string]types.RepoToken
	api  map[string]types.APIToken
}

func (m *tokenMemory) Create(_ context.Context, token types.RepoToken) (types.RepoToken, error) {
	if m.repo == nil {
		m.repo = map[string]types.RepoToken{}
	}
	m.repo[token.Hash] = token
	return token, nil
}
func (m *tokenMemory) GetByID(_ context.Context, id types.TokenID) (types.RepoToken, error) {
	for _, token := range m.repo {
		if token.ID == id {
			return token, nil
		}
	}
	return types.RepoToken{}, meta.ErrNotFound
}
func (m *tokenMemory) GetByHash(_ context.Context, hash string) (types.RepoToken, error) {
	token, ok := m.repo[hash]
	if !ok {
		return types.RepoToken{}, meta.ErrNotFound
	}
	return token, nil
}
func (*tokenMemory) List(context.Context, types.RepoID, types.TokenState, types.OffsetPage) ([]types.RepoToken, types.OffsetResult, error) {
	return nil, types.OffsetResult{}, nil
}
func (*tokenMemory) Revoke(context.Context, types.TokenID) error { return nil }

type apiTokenMemory struct{ *tokenMemory }

func (m apiTokenMemory) Create(_ context.Context, token types.APIToken) (types.APIToken, error) {
	if m.api == nil {
		m.api = map[string]types.APIToken{}
	}
	m.api[token.Hash] = token
	return token, nil
}
func (m apiTokenMemory) GetByHash(_ context.Context, hash string) (types.APIToken, error) {
	token, ok := m.api[hash]
	if !ok {
		return types.APIToken{}, meta.ErrNotFound
	}
	return token, nil
}

func TestRepoAuthorizerScopeStateRepoAndExpiry(t *testing.T) {
	now := time.Unix(2_000_000_000, 0).UTC()
	plain, hash, _, expires, err := MintRepo(types.ScopeRead, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	memory := &tokenMemory{repo: map[string]types.RepoToken{hash: {
		RepoID: "repo_1", Hash: hash, Scope: types.ScopeRead, State: types.TokenActive, ExpiresAt: expires,
	}}}
	authorizer := NewRepoAuthorizer(memory, func() time.Time { return now })
	repo := types.Repo{ID: "repo_1", AccountID: "a"}
	if err := authorizer.Authorize(context.Background(), repo, plain, false); err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), repo, plain, true); err == nil {
		t.Fatal("read credential wrote")
	}
	if err := authorizer.Authorize(context.Background(), types.Repo{ID: "repo_2"}, plain, false); !errors.Is(err, ErrMismatch) {
		t.Fatalf("wrong repo: %v", err)
	}
	token := memory.repo[hash]
	token.State = types.TokenRevoked
	memory.repo[hash] = token
	if err := authorizer.Authorize(context.Background(), repo, plain, false); !errors.Is(err, ErrMismatch) {
		t.Fatalf("revoked: %v", err)
	}
	token.State, token.ExpiresAt = types.TokenActive, now
	memory.repo[hash] = token
	if err := authorizer.Authorize(context.Background(), repo, plain, false); !errors.Is(err, ErrExpired) {
		t.Fatalf("stored expiry: %v", err)
	}
	if err := authorizer.Authorize(context.Background(), repo, "bad", false); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("malformed: %v", err)
	}
}

func TestCredentialAndControlIssuers(t *testing.T) {
	now := time.Unix(2_000_000_000, 0).UTC()
	memory := &tokenMemory{}
	issuer := NewCredentialIssuer(memory, func() time.Time { return now })
	issued, err := issuer.Issue(context.Background(), "repo_1", types.ScopeWrite, 60)
	if err != nil || issued.Plaintext == "" || len(memory.repo) != 1 {
		t.Fatalf("issued %+v %v", issued, err)
	}
	plain, hash, err := MintAPI()
	if err != nil || plain == "" || hash != HashAPI(plain) {
		t.Fatalf("api mint %q %v", plain, err)
	}
	apiMemory := apiTokenMemory{memory}
	_, _ = apiMemory.Create(context.Background(), types.APIToken{AccountID: "a", Hash: hash})
	authorizer := NewControlAuthorizer(apiMemory)
	if err := authorizer.Authorize(context.Background(), "a", plain); err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), "b", plain); !errors.Is(err, ErrMismatch) {
		t.Fatalf("wrong tenant: %v", err)
	}
	if err := authorizer.Authorize(context.Background(), "a", "other"); err == nil {
		t.Fatal("accepted unknown api token")
	}
}
