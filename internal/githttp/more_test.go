package githttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestNilTokens(t *testing.T) {
	t.Parallel()
	h := New(func(string, string) (storer.Storer, error) { return nil, errors.New("x") }, nil)
	req := httptest.NewRequest(http.MethodGet, "/git/default/app.git/info/refs?service=git-upload-pack", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("%d", rec.Code)
	}
	req.Header.Set("Authorization", "Basic !!!")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad basic %d", rec.Code)
	}
}

func TestInfoRefsAndLoader(t *testing.T) {
	t.Parallel()
	openErr := func(string, string) (storer.Storer, error) { return nil, errors.New("nope") }
	h := New(openErr, staticTokens{scope: types.ScopeWrite})
	req := httptest.NewRequest(http.MethodGet, "/git/default/app.git/info/refs?service=git-foo", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("svc %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/git/default/app.git/info/refs?service=git-upload-pack", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("open %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/git/default/app.git/info/refs?service=git-upload-pack", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/git/default/app.git/git-upload-pack", strings.NewReader(""))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("upload unauth %d", rec.Code)
	}
	l := loader{open: openErr}
	ep, _ := transport.NewEndpoint("http://x/onlyone")
	if _, err := l.Load(ep); err != transport.ErrRepositoryNotFound {
		t.Fatalf("short path %v", err)
	}
	ep, _ = transport.NewEndpoint("http://x/ns/repo.git")
	if _, err := l.Load(ep); err != transport.ErrRepositoryNotFound {
		t.Fatalf("open err %v", err)
	}
}

func TestPackDecodeErrors(t *testing.T) {
	t.Parallel()
	srv := testGitServer(t, staticTokens{scope: types.ScopeWrite})
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/git/default/app.git/git-upload-pack", strings.NewReader("nope"))
	req.Header.Set("Authorization", "Bearer x")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("upload %d", resp.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/git/default/app.git/git-receive-pack", strings.NewReader("nope"))
	req.Header.Set("Authorization", "Bearer x")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("receive %d", resp.StatusCode)
	}
}
