package githttp

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/mikerudolph/artifacts/internal/types"
)

type TokenLookup interface {
	Lookup(ctx context.Context, ns, repo, plaintext string) (types.Scope, error)
}

type server struct {
	open   func(ns, repo string) (storer.Storer, error)
	tokens TokenLookup
	git    transport.Transport
}

func New(open func(ns, repo string) (storer.Storer, error), tokens TokenLookup) http.Handler {
	s := &server{
		open:   open,
		tokens: tokens,
		git:    gitserver.NewServer(loader{open: open}),
	}
	r := chi.NewRouter()
	r.Get("/git/{ns}/{repo}/info/refs", s.infoRefs)
	r.Post("/git/{ns}/{repo}/git-upload-pack", s.uploadPack)
	r.Post("/git/{ns}/{repo}/git-receive-pack", s.receivePack)
	return r
}

func repoName(r *http.Request) (ns, repo string) {
	return chi.URLParam(r, "ns"), strings.TrimSuffix(chi.URLParam(r, "repo"), ".git")
}

func bearerOrBasic(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if strings.HasPrefix(strings.ToLower(h), "basic ") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h[6:]))
		if err != nil {
			return ""
		}
		_, pass, _ := strings.Cut(string(raw), ":")
		return pass
	}
	return ""
}

type loader struct {
	open func(ns, repo string) (storer.Storer, error)
}

func (l loader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	parts := strings.Split(strings.Trim(ep.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, transport.ErrRepositoryNotFound
	}
	st, err := l.open(parts[0], strings.TrimSuffix(parts[1], ".git"))
	if err != nil {
		return nil, transport.ErrRepositoryNotFound
	}
	return st, nil
}
