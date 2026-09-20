package types

import "time"

type RepoToken struct {
	ID        TokenID    `json:"id"`
	RepoID    RepoID     `json:"-"`
	Hash      string     `json:"-"`
	Scope     Scope      `json:"scope"`
	State     TokenState `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

type CreateTokenInput struct {
	Repo  RepoName `json:"repo"`
	Scope Scope    `json:"scope"`
	TTL   int      `json:"ttl"`
}

type CreateTokenResult struct {
	ID        TokenID   `json:"id"`
	Plaintext string    `json:"plaintext"`
	Scope     Scope     `json:"scope"`
	ExpiresAt time.Time `json:"expires_at"`
}

type APIToken struct {
	ID        string    `json:"id"`
	AccountID AccountID `json:"-"`
	Hash      string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
