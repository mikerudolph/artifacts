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

type ControlAuthorizer interface {
	Authorize(ctx context.Context, account types.AccountID, plaintext string) error
}

type Dependencies struct {
	RepoAuthorizer interface {
		Authorize(context.Context, types.Repo, string, bool) error
	}
	ReadGit    func(context.Context, string, string, string, func(storer.Storer) error) error
	Jobs       *jobs.Runner
	Authorizer ControlAuthorizer
	Repository RepositoryContent
	StreamIdle time.Duration
}

type RepositoryContent interface {
	Commit(context.Context, types.Repo, types.CommitInput) (types.CommitResult, error)
	Refs(context.Context, types.Repo) ([]types.Ref, error)
	WAL(context.Context, types.Repo) ([]types.PackWAL, error)
}

func New(svc *service.Services, cfg config.Config) http.Handler {
	return NewWithDependencies(svc, cfg, Dependencies{})
}

func NewWithDependencies(svc *service.Services, cfg config.Config, deps Dependencies) http.Handler {
	s := &server{svc: svc, cfg: cfg, deps: deps}
	r := chi.NewRouter()
	r.Route("/client/v4/accounts/{account_id}/artifacts", func(r chi.Router) {
		r.With(s.auth).Post("/namespaces", s.createNamespace)
		r.With(s.auth).Get("/namespaces", s.listNamespaces)
		r.With(s.auth).Get("/namespaces/{namespace}", s.getNamespace)
		r.With(s.auth).Post("/namespaces/{namespace}/repos", s.createRepo)
		r.With(s.auth).Get("/namespaces/{namespace}/repos", s.listRepos)
		r.With(s.auth).Get("/namespaces/{namespace}/repos/{name}", s.getRepo)
		r.With(s.auth).Delete("/namespaces/{namespace}/repos/{name}", s.deleteRepo)
		r.With(s.auth).Get("/namespaces/{namespace}/repos/{name}/tokens", s.listTokens)
		r.With(s.auth).Post("/namespaces/{namespace}/tokens", s.createToken)
		r.With(s.auth).Delete("/namespaces/{namespace}/tokens/{id}", s.revokeToken)
		r.With(s.auth).Get("/namespaces/{namespace}/repos/{name}/credentials", s.listTokens)
		r.With(s.auth).Post("/namespaces/{namespace}/credentials", s.createToken)
		r.With(s.auth).Delete("/namespaces/{namespace}/credentials/{id}", s.revokeToken)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/log", s.handleLog)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/commit/{hash}", s.handleCommit)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/tree", s.handleTreeAt)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/tree/{hash}", s.handleTree)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/blob/{hash}", s.handleBlob)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/file", s.handleFile)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/raw/{ref}/*", s.handleRaw)
		r.With(s.auth).Post("/namespaces/{namespace}/repos/{name}/fork", s.handleFork)
		r.With(s.auth).Post("/namespaces/{namespace}/repos/{name}/import", s.handleImport)
		r.With(s.contentAuth).Post("/namespaces/{namespace}/repos/{name}/commits", s.createCommit)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/refs", s.listRefs)
		r.With(s.contentAuth).Get("/namespaces/{namespace}/repos/{name}/events", s.listEvents)
		r.With(s.auth).Get("/namespaces/{namespace}/repos/{name}/wal", s.listWAL)
		r.With(s.auth).Patch("/namespaces/{namespace}/repos/{name}/settings", s.updateSettings)
		r.With(s.auth).Get("/namespaces/{namespace}/repos/{name}/jobs", s.listJobs)
	})
	return r
}
