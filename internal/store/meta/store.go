package meta

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

// Store is the metadata database. Implementations must honor RunInTx.
type Store interface {
	Accounts() Accounts
	Namespaces() Namespaces
	Repos() Repos
	Refs() Refs
	RepoTokens() RepoTokens
	APITokens() APITokens
	Jobs() Jobs
	RunInTx(ctx context.Context, fn func(Store) error) error
}

// V2Store adds immutable publication, checkpoint, and snapshot-fork metadata.
type V2Store interface {
	Store
	WAL() WAL
	Checkpoints() Checkpoints
	Forks() Forks
}

// WAL is the Postgres publication authority for immutable pack writes.
type WAL interface {
	Publish(ctx context.Context, publication types.Publication) (int64, error)
	List(ctx context.Context, repoID types.RepoID, after, through int64) ([]types.PackWAL, error)
}

// Checkpoints persists disposable-cache rebuild anchors.
type Checkpoints interface {
	Put(ctx context.Context, checkpoint types.Checkpoint) error
	Get(ctx context.Context, repoID types.RepoID) (types.Checkpoint, error)
}

// Forks creates and resolves metadata-only snapshot lineage.
type Forks interface {
	CreateSnapshot(ctx context.Context, source types.RepoID, dest types.Repo, defaultOnly bool) (types.Repo, error)
	Get(ctx context.Context, repoID types.RepoID) (types.ForkLineage, error)
	Children(ctx context.Context, repoID types.RepoID) ([]types.ForkLineage, error)
}

// Accounts persists tenants.
type Accounts interface {
	Ensure(ctx context.Context, id types.AccountID) error
	Get(ctx context.Context, id types.AccountID) (types.AccountID, error)
}

// Namespaces persists namespace records.
type Namespaces interface {
	Create(ctx context.Context, ns types.Namespace) (types.Namespace, error)
	GetByName(ctx context.Context, accountID types.AccountID, name types.NamespaceName) (types.Namespace, error)
	List(ctx context.Context, accountID types.AccountID, page types.CursorPage) ([]types.Namespace, types.CursorResult, error)
}

// Repos persists repository records.
type Repos interface {
	Create(ctx context.Context, repo types.Repo) (types.Repo, error)
	GetByName(ctx context.Context, namespaceID types.NamespaceID, name types.RepoName) (types.Repo, error)
	GetByID(ctx context.Context, id types.RepoID) (types.Repo, error)
	List(ctx context.Context, opts ListReposOpts) ([]types.Repo, types.CursorResult, error)
	Update(ctx context.Context, repo types.Repo) (types.Repo, error)
	Transition(ctx context.Context, id types.RepoID, from, to types.RepoStatus, deletedAt *time.Time) (types.Repo, error)
	Delete(ctx context.Context, id types.RepoID) error
}

// ListReposOpts filters and pages repository lists.
type ListReposOpts struct {
	NamespaceID types.NamespaceID
	Search      string
	Sort        types.RepoSortField
	Direction   types.SortDirection
	Page        types.CursorPage
}

// Refs persists git refs with compare-and-swap.
type Refs interface {
	Get(ctx context.Context, repoID types.RepoID, name string) (types.Ref, error)
	List(ctx context.Context, repoID types.RepoID) ([]types.Ref, error)
	CompareAndSwap(ctx context.Context, repoID types.RepoID, name, oldSHA, newSHA string) error
	DeleteAll(ctx context.Context, repoID types.RepoID) error
}

// RepoTokens persists hashed git tokens.
type RepoTokens interface {
	Create(ctx context.Context, tok types.RepoToken) (types.RepoToken, error)
	GetByID(ctx context.Context, id types.TokenID) (types.RepoToken, error)
	GetByHash(ctx context.Context, hash string) (types.RepoToken, error)
	List(ctx context.Context, repoID types.RepoID, state types.TokenState, page types.OffsetPage) ([]types.RepoToken, types.OffsetResult, error)
	Revoke(ctx context.Context, id types.TokenID) error
}

// APITokens persists hashed control-plane tokens.
type APITokens interface {
	Create(ctx context.Context, tok types.APIToken) (types.APIToken, error)
	GetByHash(ctx context.Context, hash string) (types.APIToken, error)
}

// Jobs persists background import, fork, and delete jobs.
type Jobs interface {
	Create(ctx context.Context, job types.Job) (types.Job, error)
	Get(ctx context.Context, id types.JobID) (types.Job, error)
	Update(ctx context.Context, job types.Job) (types.Job, error)
	ListByRepo(ctx context.Context, repoID types.RepoID) ([]types.Job, error)
}
