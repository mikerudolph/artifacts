package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/service"
)

type server struct {
	svc *service.Services
	cfg config.Config
}

// New mounts the Cloudflare-compatible control plane.
func New(svc *service.Services, cfg config.Config) http.Handler {
	s := &server{svc: svc, cfg: cfg}
	active = s
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
		registerContent(r)
		registerJobs(r)
	})
	return r
}
