package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/types"
)

type server struct {
	svc  *service.Services
	cfg  config.Config
	deps Dependencies
}

// ControlAuthorizer validates a tenant-bound control-plane credential.
type ControlAuthorizer interface {
	Authorize(ctx context.Context, account types.AccountID, plaintext string) error
}

// Dependencies are request-scoped API adapters.
type Dependencies struct {
	ReadGit    func(context.Context, string, string, string, func(storer.Storer) error) error
	Jobs       *jobs.Runner
	Authorizer ControlAuthorizer
	Repository RepositoryContent
	StreamIdle time.Duration
}

// RepositoryContent exposes publication state to REST.
type RepositoryContent interface {
	Commit(context.Context, types.Repo, types.CommitInput) (types.CommitResult, error)
	Refs(context.Context, types.Repo) ([]types.Ref, error)
	WAL(context.Context, types.Repo) ([]types.PackWAL, error)
}

// New mounts the Cloudflare-compatible control plane.
func New(svc *service.Services, cfg config.Config) http.Handler {
	return NewWithDependencies(svc, cfg, Dependencies{})
}

// NewWithDependencies mounts the control plane without process globals.
func NewWithDependencies(svc *service.Services, cfg config.Config, deps Dependencies) http.Handler {
	s := &server{svc: svc, cfg: cfg, deps: deps}
	r := chi.NewRouter()
	r.Route("/client/v4/accounts/{account_id}/artifacts", func(r chi.Router) {
		r.Use(s.auth)
		r.Post("/namespaces", s.createNamespace)
		r.Get("/namespaces", s.listNamespaces)
		r.Get("/namespaces/{namespace}", s.getNamespace)
		r.Post("/namespaces/{namespace}/repos", s.createRepo)
		r.Get("/namespaces/{namespace}/repos", s.listRepos)
		r.Get("/namespaces/{namespace}/repos/{name}", s.getRepo)
		r.Delete("/namespaces/{namespace}/repos/{name}", s.deleteRepo)
		r.Get("/namespaces/{namespace}/repos/{name}/tokens", s.listTokens)
		r.Post("/namespaces/{namespace}/tokens", s.createToken)
		r.Delete("/namespaces/{namespace}/tokens/{id}", s.revokeToken)
		r.Get("/namespaces/{namespace}/repos/{name}/credentials", s.listTokens)
		r.Post("/namespaces/{namespace}/credentials", s.createToken)
		r.Delete("/namespaces/{namespace}/credentials/{id}", s.revokeToken)
		r.Get("/namespaces/{namespace}/repos/{name}/log", s.handleLog)
		r.Get("/namespaces/{namespace}/repos/{name}/commit/{hash}", s.handleCommit)
		r.Get("/namespaces/{namespace}/repos/{name}/tree", s.handleTreeAt)
		r.Get("/namespaces/{namespace}/repos/{name}/tree/{hash}", s.handleTree)
		r.Get("/namespaces/{namespace}/repos/{name}/blob/{hash}", s.handleBlob)
		r.Get("/namespaces/{namespace}/repos/{name}/file", s.handleFile)
		r.Get("/namespaces/{namespace}/repos/{name}/raw/{ref}/*", s.handleRaw)
		r.Post("/namespaces/{namespace}/repos/{name}/fork", s.handleFork)
		r.Post("/namespaces/{namespace}/repos/{name}/import", s.handleImport)
		r.Post("/namespaces/{namespace}/repos/{name}/commits", s.createCommit)
		r.Get("/namespaces/{namespace}/repos/{name}/refs", s.listRefs)
		r.Get("/namespaces/{namespace}/repos/{name}/wal", s.listWAL)
		r.Patch("/namespaces/{namespace}/repos/{name}/settings", s.updateSettings)
		r.Get("/namespaces/{namespace}/repos/{name}/jobs", s.listJobs)
	})
	return r
}
