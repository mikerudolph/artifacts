package meta

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

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

type V2Store interface {
	Store
	WAL() WAL
	Checkpoints() Checkpoints
	Forks() Forks
}

type WAL interface {
	Publish(ctx context.Context, publication types.Publication) (int64, error)
	List(ctx context.Context, repoID types.RepoID, after, through int64) ([]types.PackWAL, error)
}

type Checkpoints interface {
	Put(ctx context.Context, checkpoint types.Checkpoint) error
	Get(ctx context.Context, repoID types.RepoID) (types.Checkpoint, error)
}

type Forks interface {
	CreateSnapshot(ctx context.Context, source types.RepoID, dest types.Repo, defaultOnly bool) (types.Repo, error)
	Get(ctx context.Context, repoID types.RepoID) (types.ForkLineage, error)
	Children(ctx context.Context, repoID types.RepoID) ([]types.ForkLineage, error)
}

type Accounts interface {
	Ensure(ctx context.Context, id types.AccountID) error
	Get(ctx context.Context, id types.AccountID) (types.AccountID, error)
}

type Namespaces interface {
	Create(ctx context.Context, ns types.Namespace) (types.Namespace, error)
	GetByName(ctx context.Context, accountID types.AccountID, name types.NamespaceName) (types.Namespace, error)
	List(ctx context.Context, accountID types.AccountID, page types.CursorPage) ([]types.Namespace, types.CursorResult, error)
}

type Repos interface {
	Create(ctx context.Context, repo types.Repo) (types.Repo, error)
	GetByName(ctx context.Context, namespaceID types.NamespaceID, name types.RepoName) (types.Repo, error)
	GetByID(ctx context.Context, id types.RepoID) (types.Repo, error)
	List(ctx context.Context, opts ListReposOpts) ([]types.Repo, types.CursorResult, error)
	Update(ctx context.Context, repo types.Repo) (types.Repo, error)
	Transition(ctx context.Context, id types.RepoID, from, to types.RepoStatus, deletedAt *time.Time) (types.Repo, error)
	Delete(ctx context.Context, id types.RepoID) error
}

type ListReposOpts struct {
	NamespaceID types.NamespaceID
	Search      string
	Sort        types.RepoSortField
	Direction   types.SortDirection
	Page        types.CursorPage
}

type Refs interface {
	Get(ctx context.Context, repoID types.RepoID, name string) (types.Ref, error)
	List(ctx context.Context, repoID types.RepoID) ([]types.Ref, error)
	CompareAndSwap(ctx context.Context, repoID types.RepoID, name, oldSHA, newSHA string) error
	DeleteAll(ctx context.Context, repoID types.RepoID) error
}

type RepoTokens interface {
	Create(ctx context.Context, tok types.RepoToken) (types.RepoToken, error)
	GetByID(ctx context.Context, id types.TokenID) (types.RepoToken, error)
	GetByHash(ctx context.Context, hash string) (types.RepoToken, error)
	List(ctx context.Context, repoID types.RepoID, state types.TokenState, page types.OffsetPage) ([]types.RepoToken, types.OffsetResult, error)
	Revoke(ctx context.Context, id types.TokenID) error
}

type APITokens interface {
	Create(ctx context.Context, tok types.APIToken) (types.APIToken, error)
	GetByHash(ctx context.Context, hash string) (types.APIToken, error)
}

type Jobs interface {
	Create(ctx context.Context, job types.Job) (types.Job, error)
	Get(ctx context.Context, id types.JobID) (types.Job, error)
	Update(ctx context.Context, job types.Job) (types.Job, error)
	ListByRepo(ctx context.Context, repoID types.RepoID) ([]types.Job, error)
}
