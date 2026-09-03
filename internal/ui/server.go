// Package ui serves the local no-auth repository browser.
package ui

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"net/http"
	pathpkg "path"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/types"
)

const maxPreviewBytes = 256 << 10

type server struct {
	rest     restClient
	template *template.Template
}

type page struct {
	View        string
	Account     string
	Namespace   string
	Repo        types.Repo
	Namespaces  []types.Namespace
	Repos       []types.Repo
	Branches    []branchOption
	Entries     []browserEntry
	Commits     []types.LogEntry
	WAL         []types.PackWAL
	Ref         string
	Path        string
	ParentURL   string
	Commit      string
	FileName    string
	FileContent string
	ContentType string
	Previewable bool
	TooLarge    bool
	DownloadURL string
	Error       string
}

type branchOption struct {
	Name     string
	Selected bool
}

type browserEntry struct {
	Mode string
	Type string
	Name string
	Href string
}

//go:embed templates/*.html assets/*.css
var files embed.FS

// New constructs a local browser backed only by the REST handler.
func New(rest http.Handler) (http.Handler, error) {
	if rest == nil {
		return nil, errors.New("REST handler is required")
	}
	funcs := template.FuncMap{"short": shortHash}
	tmpl, err := template.New("browser").Funcs(funcs).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &server{rest: restClient{handler: rest}, template: tmpl}
	router := chi.NewRouter()
	router.Get("/assets/style.css", s.style)
	router.Get("/", s.landing)
	router.Get("/{account}", s.namespaces)
	router.Get("/{account}/{namespace}", s.repos)
	router.Get("/{account}/{namespace}/{repo}", s.code)
	router.Get("/{account}/{namespace}/{repo}/browse/*", s.code)
	router.Get("/{account}/{namespace}/{repo}/file/*", s.file)
	router.Get("/{account}/{namespace}/{repo}/download/*", s.download)
	router.Get("/{account}/{namespace}/{repo}/commits", s.commits)
	router.Get("/{account}/{namespace}/{repo}/wal", s.wal)
	router.Get("/{account}/{namespace}/{repo}/settings", s.settings)
	return router, nil
}

func shortHash(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func (s *server) render(w http.ResponseWriter, data page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := s.template.ExecuteTemplate(w, "page.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) style(w http.ResponseWriter, _ *http.Request) {
	body, _ := files.ReadFile("assets/style.css")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

func (s *server) landing(w http.ResponseWriter, _ *http.Request) {
	s.render(w, page{View: "landing"})
}

func (s *server) namespaces(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	data := page{View: "namespaces", Account: account}
	data.Namespaces, data.Error = s.rest.namespaces(r.Context(), account)
	s.render(w, data)
}

func (s *server) repos(w http.ResponseWriter, r *http.Request) {
	account, namespace := chi.URLParam(r, "account"), chi.URLParam(r, "namespace")
	data := page{View: "repos", Account: account, Namespace: namespace}
	data.Repos, data.Error = s.rest.repos(r.Context(), account, namespace)
	s.render(w, data)
}

func (s *server) repositoryPage(r *http.Request, view string) page {
	account := chi.URLParam(r, "account")
	namespace := chi.URLParam(r, "namespace")
	repoName := chi.URLParam(r, "repo")
	data := page{View: view, Account: account, Namespace: namespace}
	repo, err := s.rest.repo(r.Context(), account, namespace, repoName)
	if err != nil {
		data.Error = err.Error()
		return data
	}
	data.Repo = repo
	data.Ref = r.URL.Query().Get("ref")
	if data.Ref == "" {
		data.Ref = repo.DefaultBranch
	}
	return data
}

func (s *server) code(w http.ResponseWriter, r *http.Request) {
	data := s.repositoryPage(r, "code")
	if data.Error != "" {
		s.render(w, data)
		return
	}
	refs, err := s.rest.refs(r.Context(), data.Account, data.Namespace, string(data.Repo.Name))
	if err != nil {
		data.Error = "load refs: " + err.Error()
		s.render(w, data)
		return
	}
	data.Branches = branchOptions(refs, data.Ref)
	data.Path = strings.Trim(chi.URLParam(r, "*"), "/")
	tree, err := s.rest.tree(r.Context(), data.Account, data.Namespace, string(data.Repo.Name), data.Ref, data.Path)
	if err != nil {
		if len(refs) != 0 || !isRESTStatus(err, http.StatusNotFound) {
			data.Error = "load tree: " + err.Error()
		}
		s.render(w, data)
		return
	}
	data.Commit = tree.Commit
	data.Path = tree.Path
	data.Entries = browserEntries(data, tree.Entries)
	data.ParentURL = parentURL(data)
	s.render(w, data)
}

func branchOptions(refs []types.Ref, selected string) []branchOption {
	var branches []branchOption
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(ref.Name, "refs/heads/")
		branches = append(branches, branchOption{Name: name, Selected: name == selected})
	}
	if len(branches) == 0 && selected != "" {
		branches = append(branches, branchOption{Name: selected, Selected: true})
	}
	return branches
}

func browserEntries(data page, entries []types.TreeEntry) []browserEntry {
	result := make([]browserEntry, 0, len(entries))
	base := browserRepoPath(data.Account, data.Namespace, string(data.Repo.Name))
	for _, entry := range entries {
		child := entry.Name
		if data.Path != "" {
			child = data.Path + "/" + entry.Name
		}
		kind := "file"
		if entry.Type == "tree" {
			kind = "browse"
		}
		href := base + "/" + kind + "/" + escapeRepositoryPath(child)
		result = append(result, browserEntry{Mode: entry.Mode, Type: entry.Type, Name: entry.Name, Href: withRef(href, data.Ref)})
	}
	return result
}

func parentURL(data page) string {
	if data.Path == "" {
		return ""
	}
	base := browserRepoPath(data.Account, data.Namespace, string(data.Repo.Name))
	parent := pathpkg.Dir(data.Path)
	if parent != "." {
		base += "/browse/" + escapeRepositoryPath(parent)
	}
	return withRef(base, data.Ref)
}

func (s *server) file(w http.ResponseWriter, r *http.Request) {
	data := s.repositoryPage(r, "file")
	data.Path = strings.Trim(chi.URLParam(r, "*"), "/")
	data.FileName = pathpkg.Base(data.Path)
	data.ParentURL = fileParentURL(data)
	data.DownloadURL = downloadURL(data)
	if data.Error == "" {
		body, err := s.rest.file(r.Context(), data.Account, data.Namespace, string(data.Repo.Name), data.Ref, data.Path)
		switch {
		case errors.Is(err, errResponseTooLarge):
			data.TooLarge = true
		case err != nil:
			data.Error = err.Error()
		default:
			data.ContentType = http.DetectContentType(body)
			data.Previewable = canPreview(body, data.ContentType)
			if data.Previewable {
				data.FileContent = string(body)
			}
		}
	}
	s.render(w, data)
}

func canPreview(body []byte, contentType string) bool {
	if !utf8.Valid(body) || bytes.ContainsRune(body, 0) {
		return false
	}
	mediaType := strings.SplitN(contentType, ";", 2)[0]
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" ||
		strings.HasSuffix(mediaType, "+json") || mediaType == "application/xml" || strings.HasSuffix(mediaType, "+xml")
}

func fileParentURL(data page) string {
	base := browserRepoPath(data.Account, data.Namespace, string(data.Repo.Name))
	parent := pathpkg.Dir(data.Path)
	if parent != "." {
		base += "/browse/" + escapeRepositoryPath(parent)
	}
	return withRef(base, data.Ref)
}

func downloadURL(data page) string {
	base := browserRepoPath(data.Account, data.Namespace, string(data.Repo.Name))
	return withRef(base+"/download/"+escapeRepositoryPath(data.Path), data.Ref)
}

func (s *server) download(w http.ResponseWriter, r *http.Request) {
	file := fileRequest{
		account: chi.URLParam(r, "account"), namespace: chi.URLParam(r, "namespace"), repo: chi.URLParam(r, "repo"),
		ref: r.URL.Query().Get("ref"), path: strings.Trim(chi.URLParam(r, "*"), "/"),
	}
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	s.rest.serveFile(w, r.Context(), file)
}

func (s *server) commits(w http.ResponseWriter, r *http.Request) {
	data := s.repositoryPage(r, "commits")
	if data.Error == "" {
		refs, err := s.rest.refs(r.Context(), data.Account, data.Namespace, string(data.Repo.Name))
		if err == nil {
			data.Branches = branchOptions(refs, data.Ref)
			data.Commits, err = s.rest.log(r.Context(), data.Account, data.Namespace, string(data.Repo.Name), data.Ref)
		}
		if err != nil && (len(refs) != 0 || !isRESTStatus(err, http.StatusNotFound)) {
			data.Error = err.Error()
		}
	}
	s.render(w, data)
}

func (s *server) wal(w http.ResponseWriter, r *http.Request) {
	data := s.repositoryPage(r, "wal")
	if data.Error == "" {
		var err error
		data.WAL, err = s.rest.wal(r.Context(), data.Account, data.Namespace, string(data.Repo.Name))
		if err != nil {
			data.Error = err.Error()
		}
	}
	s.render(w, data)
}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	s.render(w, s.repositoryPage(r, "settings"))
}
