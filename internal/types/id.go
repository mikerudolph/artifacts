package types

// AccountID is a tenant identifier (Cloudflare account id or "local").
type AccountID string

// NamespaceID is the durable primary key for a namespace.
type NamespaceID string

// RepoID is the durable primary key for a repository.
type RepoID string

// TokenID is the durable primary key for a repo-scoped token.
type TokenID string

// JobID is the durable primary key for a background job.
type JobID string
