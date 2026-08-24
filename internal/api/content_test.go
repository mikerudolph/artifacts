package api

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	h := testAPI(t, "none", "")
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	st, commit, tree, blob := seedContent(t)
	OpenGit = func(string, string, string) (storer.Storer, error) { return st, nil }
	t.Cleanup(func() { OpenGit = nil })
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

func rawGet(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func seedContent(t *testing.T) (storer.Storer, plumbing.Hash, plumbing.Hash, plumbing.Hash) {
	t.Helper()
	raw, err := gitstore.Open(objecttest.NewMem(), &contentRefs{data: map[string]string{}}, "local", "repo_1")
	if err != nil {
		t.Fatal(err)
	}
	bh := putBlobObj(t, raw, "hello")
	th := putTreeObj(t, raw, bh)
	ch := putCommitObj(t, raw, th)
	if err := raw.SetReference(plumbing.NewHashReference("refs/heads/main", ch)); err != nil {
		t.Fatal(err)
	}
	return raw, ch, th, bh
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
