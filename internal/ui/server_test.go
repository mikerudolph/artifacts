package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type browserData struct{}

func (browserData) ListNamespaces(context.Context, types.AccountID, types.CursorPage) ([]types.Namespace, types.CursorResult, error) {
	return []types.Namespace{{Name: "safe<script>"}}, types.CursorResult{}, nil
}

type failingData struct{}

func (failingData) ListNamespaces(context.Context, types.AccountID, types.CursorPage) ([]types.Namespace, types.CursorResult, error) {
	return nil, types.CursorResult{}, errors.New("namespace failure")
}
func (failingData) ListRepos(context.Context, types.AccountID, string, meta.ListReposOpts) ([]types.Repo, types.CursorResult, error) {
	return nil, types.CursorResult{}, errors.New("repo list failure")
}
func (failingData) GetRepo(context.Context, types.AccountID, string, string) (types.Repo, error) {
	return types.Repo{}, errors.New("repo failure")
}

type failingReader struct{}

func (failingReader) Read(context.Context, types.Repo, func(storer.Storer) error) error {
	return errors.New("open failure")
}
func (failingReader) Refs(context.Context, types.Repo) ([]types.Ref, error) {
	return nil, errors.New("refs failure")
}
func (failingReader) WAL(context.Context, types.Repo) ([]types.PackWAL, error) {
	return nil, errors.New("wal failure")
}
func (browserData) ListRepos(context.Context, types.AccountID, string, meta.ListReposOpts) ([]types.Repo, types.CursorResult, error) {
	return []types.Repo{{Name: "app", Description: "agent artifacts"}}, types.CursorResult{}, nil
}
func (browserData) GetRepo(_ context.Context, account types.AccountID, ns, name string) (types.Repo, error) {
	return types.Repo{ID: "repo_1", AccountID: account, Namespace: types.NamespaceName(ns), Name: types.RepoName(name),
		Description: "<script>alert(1)</script>", DefaultBranch: "main", Remote: "http://local/git/a/n/r.git",
		StorageVersion: 2, WALSequence: 1}, nil
}

type browserReader struct{ store storer.Storer }

func (r browserReader) Read(_ context.Context, _ types.Repo, visit func(storer.Storer) error) error {
	if r.store == nil {
		r.store = memory.NewStorage()
	}
	return visit(r.store)
}
func (browserReader) Refs(context.Context, types.Repo) ([]types.Ref, error) {
	return []types.Ref{{Name: "refs/heads/main", SHA: strings.Repeat("a", 40)}}, nil
}
func (browserReader) WAL(context.Context, types.Repo) ([]types.PackWAL, error) {
	return []types.PackWAL{{Sequence: 1, Checksum: strings.Repeat("b", 64), Size: 42}}, nil
}

func TestBrowserRoutesAndEscaping(t *testing.T) {
	reader := browserReader{store: browserStore(t)}
	h, err := New(browserData{}, reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"/":                           {"Local artifact repositories"},
		"/local":                      {"Namespaces", "safe&lt;script&gt;"},
		"/local/default":              {"agent artifacts"},
		"/local/default/app":          {"Code", "Branch", "README.md"},
		"/local/default/app/commits":  {"Commits", "initial"},
		"/local/default/app/wal":      {"WAL", "42"},
		"/local/default/app/settings": {"Storage version", "2"},
		"/assets/style.css":           {"color-scheme"},
	}
	for path, fragments := range cases {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status %d", w.Code)
			}
			for _, fragment := range fragments {
				if !strings.Contains(w.Body.String(), fragment) {
					t.Fatalf("missing %q in %q", fragment, w.Body.String())
				}
			}
			if strings.Contains(w.Body.String(), "<script>alert") {
				t.Fatal("unescaped repository content")
			}
		})
	}
}

func TestBrowserErrorRoutes(t *testing.T) {
	h, err := New(failingData{}, failingReader{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/local", "/local/default", "/local/default/app", "/local/default/app/commits", "/local/default/app/wal", "/local/default/app/settings"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "failure") {
			t.Fatalf("%s: %d %q", path, w.Code, w.Body.String())
		}
	}
	h, err = New(browserData{}, failingReader{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/local/default/app", "/local/default/app/commits", "/local/default/app/wal"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if !strings.Contains(w.Body.String(), "failure") {
			t.Fatalf("reader %s: %q", path, w.Body.String())
		}
	}
}

func browserStore(t *testing.T) storer.Storer {
	t.Helper()
	store := memory.NewStorage()
	blob := store.NewEncodedObject()
	blob.SetType(plumbing.BlobObject)
	w, _ := blob.Writer()
	_, _ = w.Write([]byte("hello"))
	_ = w.Close()
	blobHash, err := store.SetEncodedObject(blob)
	if err != nil {
		t.Fatal(err)
	}
	treeObj := store.NewEncodedObject()
	tree := gitobject.Tree{Entries: []gitobject.TreeEntry{{Name: "README.md", Mode: filemode.Regular, Hash: blobHash}}}
	if err := tree.Encode(treeObj); err != nil {
		t.Fatal(err)
	}
	treeHash, _ := store.SetEncodedObject(treeObj)
	commitObj := store.NewEncodedObject()
	commit := gitobject.Commit{TreeHash: treeHash, Message: "initial\n",
		Author:    gitobject.Signature{Name: "Agent", Email: "agent@example", When: time.Now()},
		Committer: gitobject.Signature{Name: "Agent", Email: "agent@example", When: time.Now()}}
	if err := commit.Encode(commitObj); err != nil {
		t.Fatal(err)
	}
	commitHash, _ := store.SetEncodedObject(commitObj)
	if err := store.SetReference(plumbing.NewHashReference("refs/heads/main", commitHash)); err != nil {
		t.Fatal(err)
	}
	return store
}
