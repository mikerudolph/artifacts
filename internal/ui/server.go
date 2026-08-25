// Package ui serves the local no-auth repository browser.
package ui

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// RepositoryReader exposes cache-backed browser data.
type RepositoryReader interface {
	Read(context.Context, types.Repo, func(storer.Storer) error) error
	Refs(context.Context, types.Repo) ([]types.Ref, error)
	WAL(context.Context, types.Repo) ([]types.PackWAL, error)
}

// DataSource exposes tenant-scoped control-plane reads.
type DataSource interface {
	ListNamespaces(context.Context, types.AccountID, types.CursorPage) ([]types.Namespace, types.CursorResult, error)
	ListRepos(context.Context, types.AccountID, string, meta.ListReposOpts) ([]types.Repo, types.CursorResult, error)
	GetRepo(context.Context, types.AccountID, string, string) (types.Repo, error)
}

type server struct {
	services DataSource
	reader   RepositoryReader
	template *template.Template
}

type page struct {
	View       string
	Account    types.AccountID
	Namespace  string
	Repo       types.Repo
	Namespaces []types.Namespace
	Repos      []types.Repo
	Refs       []types.Ref
	WAL        []types.PackWAL
	Tree       []types.TreeEntry
	Commits    []types.LogEntry
	Error      string
}

//go:embed templates/*.html assets/*.css
var files embed.FS

// New constructs the local repository browser.
func New(services DataSource, reader RepositoryReader) (http.Handler, error) {
	funcs := template.FuncMap{
		"hasPrefix":  strings.HasPrefix,
		"trimBranch": func(name string) string { return strings.TrimPrefix(name, "refs/heads/") },
		"short": func(value string) string {
			if len(value) > 12 {
				return value[:12]
			}
			return value
		},
	}
	t, err := template.New("browser").Funcs(funcs).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &server{services: services, reader: reader, template: t}
	r := chi.NewRouter()
	r.Get("/assets/style.css", s.style)
	r.Get("/", s.landing)
	r.Get("/{account}", s.namespaces)
	r.Get("/{account}/{namespace}", s.repos)
	r.Get("/{account}/{namespace}/{repo}", s.code)
	r.Get("/{account}/{namespace}/{repo}/commits", s.commits)
	r.Get("/{account}/{namespace}/{repo}/wal", s.wal)
	r.Get("/{account}/{namespace}/{repo}/settings", s.settings)
	return r, nil
}

func (s *server) render(w http.ResponseWriter, data page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.template.ExecuteTemplate(w, "page.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) style(w http.ResponseWriter, _ *http.Request) {
	b, _ := files.ReadFile("assets/style.css")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *server) landing(w http.ResponseWriter, _ *http.Request) {
	s.render(w, page{View: "landing"})
}

func (s *server) namespaces(w http.ResponseWriter, r *http.Request) {
	account := types.AccountID(chi.URLParam(r, "account"))
	list, _, err := s.services.ListNamespaces(r.Context(), account, types.CursorPage{Limit: 200})
	if err != nil {
		s.render(w, page{View: "namespaces", Account: account, Error: err.Error()})
		return
	}
	s.render(w, page{View: "namespaces", Account: account, Namespaces: list})
}

func (s *server) repos(w http.ResponseWriter, r *http.Request) {
	account := types.AccountID(chi.URLParam(r, "account"))
	ns := chi.URLParam(r, "namespace")
	list, _, err := s.services.ListRepos(r.Context(), account, ns, meta.ListReposOpts{Page: types.CursorPage{Limit: 200}})
	if err != nil {
		s.render(w, page{View: "repos", Account: account, Namespace: ns, Error: err.Error()})
		return
	}
	s.render(w, page{View: "repos", Account: account, Namespace: ns, Repos: list})
}

func (s *server) repoPage(r *http.Request, view string) (page, error) {
	account := types.AccountID(chi.URLParam(r, "account"))
	ns, name := chi.URLParam(r, "namespace"), chi.URLParam(r, "repo")
	repo, err := s.services.GetRepo(r.Context(), account, ns, name)
	return page{View: view, Account: account, Namespace: ns, Repo: repo}, err
}

func (s *server) code(w http.ResponseWriter, r *http.Request) {
	data, err := s.repoPage(r, "code")
	if err == nil {
		data.Refs, err = s.reader.Refs(r.Context(), data.Repo)
	}
	if err == nil {
		data.Tree, err = s.tree(r.Context(), data.Repo)
	}
	if err != nil && !meta.IsNotFound(err) {
		data.Error = err.Error()
	}
	s.render(w, data)
}

func (s *server) tree(ctx context.Context, repo types.Repo) ([]types.TreeEntry, error) {
	var out []types.TreeEntry
	err := s.reader.Read(ctx, repo, func(store storer.Storer) error {
		var err error
		out, err = readTree(store, repo.DefaultBranch)
		return err
	})
	return out, err
}

func readTree(store storer.Storer, branch string) ([]types.TreeEntry, error) {
	ref, err := store.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		return nil, nil
	}
	commit, err := gitobject.GetCommit(store, ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	out := make([]types.TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		out = append(out, types.TreeEntry{Mode: entry.Mode.String(), Type: entry.Mode.String(), Hash: entry.Hash.String(), Name: entry.Name})
	}
	return out, nil
}

func (s *server) commits(w http.ResponseWriter, r *http.Request) {
	data, err := s.repoPage(r, "commits")
	if err == nil {
		data.Commits, err = s.history(r.Context(), data.Repo)
	}
	if err != nil && !meta.IsNotFound(err) {
		data.Error = err.Error()
	}
	s.render(w, data)
}

func (s *server) history(ctx context.Context, repo types.Repo) ([]types.LogEntry, error) {
	var out []types.LogEntry
	err := s.reader.Read(ctx, repo, func(store storer.Storer) error {
		var err error
		out, err = readHistory(store, repo.DefaultBranch)
		return err
	})
	return out, err
}

func readHistory(store storer.Storer, branch string) ([]types.LogEntry, error) {
	ref, err := store.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		return nil, nil
	}
	commit, err := gitobject.GetCommit(store, ref.Hash())
	if err != nil {
		return nil, err
	}
	var out []types.LogEntry
	err = gitobject.NewCommitPreorderIter(commit, nil, nil).ForEach(func(c *gitobject.Commit) error {
		out = append(out, types.LogEntry{Hash: c.Hash.String(), Message: strings.TrimSpace(c.Message),
			Author: types.Signature{Name: c.Author.Name, Email: c.Author.Email, When: c.Author.When}})
		if len(out) == 50 {
			return storer.ErrStop
		}
		return nil
	})
	if err == storer.ErrStop {
		err = nil
	}
	return out, err
}

func (s *server) wal(w http.ResponseWriter, r *http.Request) {
	data, err := s.repoPage(r, "wal")
	if err == nil {
		data.WAL, err = s.reader.WAL(r.Context(), data.Repo)
	}
	if err != nil {
		data.Error = err.Error()
	}
	s.render(w, data)
}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	data, err := s.repoPage(r, "settings")
	if err != nil {
		data.Error = err.Error()
	}
	s.render(w, data)
}
