package githttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/httpstream"
	"github.com/mikerudolph/artifacts/internal/types"
)

// RepositoryLookup resolves the tenant-qualified route to a repository.
type RepositoryLookup interface {
	Lookup(ctx context.Context, account types.AccountID, namespace, repo string) (types.Repo, error)
}

// RepositoryAuthorizer authorizes a credential against the resolved repository.
type RepositoryAuthorizer interface {
	Authorize(ctx context.Context, repo types.Repo, credential string, write bool) error
}

// RepositoryBackend serves Git RPCs from a durable repository cache.
type RepositoryBackend interface {
	RPC(ctx context.Context, repo types.Repo, service string, input io.Reader, output io.Writer, protocol string) error
	Receive(ctx context.Context, repo types.Repo, input io.Reader, protocol string) ([]byte, error)
}

type repositoryServer struct {
	backend RepositoryBackend
	lookup  RepositoryLookup
	auth    RepositoryAuthorizer
	bypass  bool
	streams chan struct{}
	idle    time.Duration
	gates   sync.Map
}

// NewRepository serves tenant-qualified smart HTTP routes.
func NewRepository(backend RepositoryBackend, lookup RepositoryLookup, authorizer RepositoryAuthorizer, bypass bool) http.Handler {
	return newRepository(backend, lookup, authorizer, bypass, httpstream.DefaultIdleTimeout)
}

// NewRepositoryWithIdleTimeout serves tenant-qualified routes with a progress deadline.
func NewRepositoryWithIdleTimeout(backend RepositoryBackend, lookup RepositoryLookup, authorizer RepositoryAuthorizer, bypass bool, idle time.Duration) http.Handler {
	return newRepository(backend, lookup, authorizer, bypass, idle)
}

func newRepository(backend RepositoryBackend, lookup RepositoryLookup, authorizer RepositoryAuthorizer, bypass bool, idle time.Duration) http.Handler {
	s := &repositoryServer{backend: backend, lookup: lookup, auth: authorizer, bypass: bypass, streams: make(chan struct{}, 32), idle: idle}
	r := chi.NewRouter()
	r.Get("/git/{account}/{namespace}/{repo}/info/refs", s.infoRefs)
	r.Post("/git/{account}/{namespace}/{repo}/git-upload-pack", s.uploadPack)
	r.Post("/git/{account}/{namespace}/{repo}/git-receive-pack", s.receivePack)
	return r
}

func (s *repositoryServer) resolve(w http.ResponseWriter, r *http.Request, write bool) (types.Repo, bool) {
	account := types.AccountID(chi.URLParam(r, "account"))
	ns := chi.URLParam(r, "namespace")
	name := strings.TrimSuffix(chi.URLParam(r, "repo"), ".git")
	repo, err := s.lookup.Lookup(r.Context(), account, ns, name)
	if err != nil || repo.Status != types.RepoReady {
		http.Error(w, "repository not found", http.StatusNotFound)
		return types.Repo{}, false
	}
	if write && repo.ReadOnly {
		http.Error(w, "repository is not writable", http.StatusForbidden)
		return types.Repo{}, false
	}
	if s.bypass {
		return repo, true
	}
	credential := bearerOrBasic(r)
	if credential == "" || s.auth == nil || s.auth.Authorize(r.Context(), repo, credential, write) != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return types.Repo{}, false
	}
	return repo, true
}

func (s *repositoryServer) infoRefs(w http.ResponseWriter, r *http.Request) {
	service := strings.TrimPrefix(r.URL.Query().Get("service"), "git-")
	write := service == "receive-pack"
	if service != "upload-pack" && !write {
		http.Error(w, "unsupported service", http.StatusForbidden)
		return
	}
	repo, ok := s.resolve(w, r, write)
	if !ok {
		return
	}
	var advertisement bytes.Buffer
	if err := s.backend.RPC(r.Context(), repo, service, nil, &advertisement, r.Header.Get("Git-Protocol")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-git-"+service+"-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	prefix := "# service=git-" + service + "\n"
	_, _ = fmt.Fprintf(w, "%04x%s0000", len(prefix)+4, prefix)
	_, _ = io.Copy(w, &advertisement)
}

func (s *repositoryServer) uploadPack(w http.ResponseWriter, r *http.Request) {
	s.serveRPC(w, r, false)
}

func (s *repositoryServer) receivePack(w http.ResponseWriter, r *http.Request) {
	s.serveRPC(w, r, true)
}

func (s *repositoryServer) serveRPC(w http.ResponseWriter, r *http.Request, write bool) {
	repo, ok := s.resolve(w, r, write)
	if !ok {
		return
	}
	release, err := s.acquireRepo(r.Context(), repo)
	if err != nil {
		http.Error(w, "stream canceled", http.StatusRequestTimeout)
		return
	}
	defer release()
	if !s.acquireStream(w) {
		return
	}
	defer func() { <-s.streams }()
	stream := httpstream.New(w, s.idle)
	defer stream.Close()
	rpcContext, cancel := context.WithCancel(r.Context())
	defer cancel()
	input := &cancelReader{Reader: stream.Reader(r.Body), cancel: cancel}
	output := &cancelWriter{Writer: stream.Writer(w), cancel: cancel}
	service := "upload-pack"
	if write {
		service = "receive-pack"
		body, err := s.backend.Receive(rpcContext, repo, input, r.Header.Get("Git-Protocol"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-git-"+service+"-result")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = output.Write(body)
		return
	}
	w.Header().Set("Content-Type", "application/x-git-"+service+"-result")
	w.Header().Set("Cache-Control", "no-cache")
	counted := &countWriter{Writer: output}
	if err := s.backend.RPC(rpcContext, repo, service, input, counted, r.Header.Get("Git-Protocol")); err != nil && counted.count == 0 {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type cancelReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r *cancelReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.cancel()
	}
	return n, err
}

type cancelWriter struct {
	io.Writer
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if err != nil {
		w.cancel()
	}
	return n, err
}

func (s *repositoryServer) acquireRepo(ctx context.Context, repo types.Repo) (func(), error) {
	key := string(repo.AccountID) + "/" + string(repo.ID)
	value, _ := s.gates.LoadOrStore(key, make(chan struct{}, 1))
	gate := value.(chan struct{})
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type countWriter struct {
	io.Writer
	count int64
}

func (w *countWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.count += int64(n)
	return n, err
}

func (s *repositoryServer) acquireStream(w http.ResponseWriter) bool {
	select {
	case s.streams <- struct{}{}:
		return true
	default:
		http.Error(w, "too many Git streams", http.StatusServiceUnavailable)
		return false
	}
}
