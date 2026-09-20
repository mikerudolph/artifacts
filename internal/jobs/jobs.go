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
	ErrInvalidURL = errors.New("invalid url")

	ErrRemoteAuth = errors.New("remote auth required")

	ErrUpstream = errors.New("upstream unavailable")

	ErrBusy = errors.New("operation in progress")
)

type Runner struct {
	meta      meta.Store
	objects   object.Store
	publicURL string
	now       func() time.Time
	imports   ImportPublisher
	upgrades  RepositoryUpgrader
	resolver  ipResolver
}

type ImportPublisher interface {
	ImportControlled(context.Context, types.Repo, types.ImportSpec) (string, error)
}

type RepositoryUpgrader interface {
	Upgrade(context.Context, types.Repo) (types.Repo, error)
}

func New(m meta.Store, objects object.Store, publicURL string) *Runner {
	return &Runner{meta: m, objects: objects, publicURL: publicURL, now: time.Now}
}

func NewWithPublisher(m meta.Store, objects object.Store, publicURL string, publisher ImportPublisher) *Runner {
	r := New(m, objects, publicURL)
	r.imports = publisher
	if upgrader, ok := publisher.(RepositoryUpgrader); ok {
		r.upgrades = upgrader
	}
	return r
}

func (r *Runner) tenantRemote(account types.AccountID, ns types.NamespaceName, repo types.RepoName) string {
	return strings.TrimRight(r.publicURL, "/") + "/git/" + string(account) + "/" + string(ns) + "/" + string(repo) + ".git"
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

func (r *Runner) List(ctx context.Context, repo types.RepoID) ([]types.Job, error) {
	return r.meta.Jobs().ListByRepo(ctx, repo)
}

func (r *Runner) startJob(ctx context.Context, repo types.RepoID, kind types.JobKind) (types.Job, error) {
	now := r.now()
	return r.meta.Jobs().Create(ctx, types.Job{
		RepoID: repo, Kind: kind, Status: types.JobRunning, Progress: 0, CreatedAt: now, UpdatedAt: now,
	})
}

func (r *Runner) finishJob(ctx context.Context, job types.Job, runErr error) error {
	job.Progress = 100
	job.Status = types.JobSucceeded
	if runErr != nil {
		job.Status = types.JobFailed
		job.Error = runErr.Error()
	}
	_, err := r.meta.Jobs().Update(ctx, job)
	if runErr != nil {
		return runErr
	}
	return err
}
