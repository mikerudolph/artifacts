package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestContentRoutes(t *testing.T) {
	st, commit, tree, blob := seedContent(t)
	h := testAPIWithDependencies(t, "none", "", Dependencies{ReadGit: fixedReader(st)})
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	base := acctBase + "/namespaces/default/repos/app"

	if rec := doJSON(t, h, http.MethodGet, base+"/log?ref=main&limit=5", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("log %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, h, http.MethodGet, base+"/commit/"+commit.String(), "", nil); rec.Code != http.StatusOK {
		t.Fatalf("commit %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, base+"/tree/"+tree.String(), "", nil); rec.Code != http.StatusOK {
		t.Fatalf("tree %d", rec.Code)
	}
	if rec := rawGet(h, base+"/blob/"+blob.String()); rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("blob %d %q", rec.Code, rec.Body.String())
	}
	if rec := rawGet(h, base+"/file?ref=main&path=README.md"); rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("file %d %q", rec.Code, rec.Body.String())
	}
	if rec := rawGet(h, base+"/raw/main/README.md"); rec.Code != http.StatusOK {
		t.Fatalf("raw %d", rec.Code)
	}
	if rec := rawGet(h, base+"/file?ref=main&path=missing"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing %d", rec.Code)
	}
}

func fixedReader(st storer.Storer) func(context.Context, string, string, string, func(storer.Storer) error) error {
	return func(_ context.Context, _, _, _ string, visit func(storer.Storer) error) error { return visit(st) }
}

func rawGet(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func seedContent(t *testing.T) (storer.Storer, plumbing.Hash, plumbing.Hash, plumbing.Hash) {
	return seedContentBody(t, "hello")
}

func seedContentBody(t *testing.T, body string) (storer.Storer, plumbing.Hash, plumbing.Hash, plumbing.Hash) {
	t.Helper()
	raw, err := gitstore.Open(objecttest.NewMem(), &contentRefs{data: map[string]string{}}, "local", "repo_1")
	if err != nil {
		t.Fatal(err)
	}
	bh := putBlobObj(t, raw, body)
	th := putTreeObj(t, raw, bh)
	ch := putCommitObj(t, raw, th)
	if err := raw.SetReference(plumbing.NewHashReference("refs/heads/main", ch)); err != nil {
		t.Fatal(err)
	}
	return raw, ch, th, bh
}

func TestStalledBlobResponseReleasesRepositoryRead(t *testing.T) {
	st, commit, _, blob := seedContentBody(t, strings.Repeat("x", 32<<20))
	var lock sync.Mutex
	started := make(chan struct{}, 2)
	reader := func(_ context.Context, _, _, _ string, visit func(storer.Storer) error) error {
		lock.Lock()
		defer lock.Unlock()
		started <- struct{}{}
		return visit(st)
	}
	h := testAPIWithDependencies(t, "none", "", Dependencies{ReadGit: reader, StreamIdle: 50 * time.Millisecond})
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(1024)
	}
	path := acctBase + "/namespaces/default/repos/app/blob/" + blob.String()
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", path, parsed.Host); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blob read did not start")
	}
	result := make(chan error, 1)
	go func() {
		response, err := http.Get(server.URL + acctBase + "/namespaces/default/repos/app/commit/" + commit.String())
		if err == nil {
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusOK {
				err = fmt.Errorf("commit status %d", response.StatusCode)
			}
		}
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stalled response retained repository read lock")
	}
}

func putBlobObj(t *testing.T, st storer.Storer, body string) plumbing.Hash {
	t.Helper()
	obj := st.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	h, err := st.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func putTreeObj(t *testing.T, st storer.Storer, blob plumbing.Hash) plumbing.Hash {
	t.Helper()
	tr := object.Tree{Entries: []object.TreeEntry{{Name: "README.md", Mode: filemode.Regular, Hash: blob}}}
	obj := st.NewEncodedObject()
	if err := tr.Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := st.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func putCommitObj(t *testing.T, st storer.Storer, tree plumbing.Hash) plumbing.Hash {
	t.Helper()
	when := time.Unix(1_700_000_000, 0).UTC()
	c := object.Commit{
		Author: object.Signature{Name: "a", Email: "a@b", When: when}, Committer: object.Signature{Name: "a", Email: "a@b", When: when},
		Message: "init\n", TreeHash: tree,
	}
	obj := st.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := st.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type contentRefs struct{ data map[string]string }

func (c *contentRefs) Get(_ context.Context, _ types.RepoID, name string) (types.Ref, error) {
	sha, ok := c.data[name]
	if !ok {
		return types.Ref{}, meta.ErrNotFound
	}
	return types.Ref{Name: name, SHA: sha}, nil
}

func (c *contentRefs) List(_ context.Context, _ types.RepoID) ([]types.Ref, error) {
	var out []types.Ref
	for n, sha := range c.data {
		out = append(out, types.Ref{Name: n, SHA: sha})
	}
	return out, nil
}

func (c *contentRefs) CompareAndSwap(_ context.Context, _ types.RepoID, name, oldSHA, newSHA string) error {
	cur, ok := c.data[name]
	if oldSHA == "" {
		if ok {
			return meta.ErrCASConflict
		}
		c.data[name] = newSHA
		return nil
	}
	if !ok || cur != oldSHA {
		return meta.ErrCASConflict
	}
	if newSHA == "" {
		delete(c.data, name)
		return nil
	}
	c.data[name] = newSHA
	return nil
}

func (c *contentRefs) DeleteAll(context.Context, types.RepoID) error {
	c.data = map[string]string{}
	return nil
}
