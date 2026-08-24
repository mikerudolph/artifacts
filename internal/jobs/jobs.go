package jobs

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

var (
	// ErrInvalidURL is a bad import source.
	ErrInvalidURL = errors.New("invalid url")
	// ErrRemoteAuth means the remote requires credentials.
	ErrRemoteAuth = errors.New("remote auth required")
	// ErrUpstream means the remote could not be reached.
	ErrUpstream = errors.New("upstream unavailable")
	// ErrBusy means the repo is importing or forking.
	ErrBusy = errors.New("operation in progress")
)

// Runner runs fork, import, and delete jobs.
type Runner struct {
	meta      meta.Store
	objects   object.Store
	publicURL string
	now       func() time.Time
}

// New constructs a Runner.
func New(m meta.Store, objects object.Store, publicURL string) *Runner {
	return &Runner{meta: m, objects: objects, publicURL: publicURL, now: time.Now}
}

func (r *Runner) remote(ns types.NamespaceName, repo types.RepoName) string {
	return strings.TrimRight(r.publicURL, "/") + "/git/" + string(ns) + "/" + string(repo) + ".git"
}

func (r *Runner) mint(ctx context.Context, repoID types.RepoID) (types.CreateTokenResult, error) {
	now := r.now()
	plain, hash, id, exp, err := auth.MintRepo(types.ScopeWrite, time.Duration(types.DefaultTTLSeconds)*time.Second, now)
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	_, err = r.meta.RepoTokens().Create(ctx, types.RepoToken{
		ID: id, RepoID: repoID, Hash: hash, Scope: types.ScopeWrite, State: types.TokenActive, CreatedAt: now, ExpiresAt: exp,
	})
	if err != nil {
		return types.CreateTokenResult{}, err
	}
	return types.CreateTokenResult{ID: id, Plaintext: plain, Scope: types.ScopeWrite, ExpiresAt: exp}, nil
}

func (r *Runner) lookup(ctx context.Context, account types.AccountID, ns, name string) (types.Repo, types.Namespace, error) {
	nsName, err := types.ParseNamespaceName(ns)
	if err != nil {
		return types.Repo{}, types.Namespace{}, err
	}
	nspace, err := r.meta.Namespaces().GetByName(ctx, account, nsName)
	if err != nil {
		return types.Repo{}, types.Namespace{}, err
	}
	repoName, err := types.ParseRepoName(name)
	if err != nil {
		return types.Repo{}, types.Namespace{}, err
	}
	repo, err := r.meta.Repos().GetByName(ctx, nspace.ID, repoName)
	return repo, nspace, err
}

func busy(status types.RepoStatus) error {
	if status == types.RepoImporting || status == types.RepoForking {
		return ErrBusy
	}
	return nil
}
