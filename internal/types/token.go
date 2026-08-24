package types

import "time"

// RepoToken is a git-scoped credential record (hash only; never store plaintext).
type RepoToken struct {
	ID        TokenID    `json:"id"`
	RepoID    RepoID     `json:"-"`
	Hash      string     `json:"-"`
	Scope     Scope      `json:"scope"`
	State     TokenState `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// CreateTokenInput is the control-plane token mint body.
type CreateTokenInput struct {
	Repo  RepoName `json:"repo"`
	Scope Scope    `json:"scope"`
	TTL   int      `json:"ttl"`
}

// CreateTokenResult is returned by POST /tokens.
type CreateTokenResult struct {
	ID        TokenID   `json:"id"`
	Plaintext string    `json:"plaintext"`
	Scope     Scope     `json:"scope"`
	ExpiresAt time.Time `json:"expires_at"`
}

// APIToken is a control-plane bearer credential (hash only).
type APIToken struct {
	ID        string    `json:"id"`
	AccountID AccountID `json:"-"`
	Hash      string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
