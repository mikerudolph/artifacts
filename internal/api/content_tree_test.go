package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/service"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestReadTreeAt(t *testing.T) {
	store, commit := seedNestedContent(t)
	tests := []struct {
		name     string
		ref      string
		path     string
		wantPath string
		wantName string
		wantErr  bool
	}{
		{name: "root", ref: "main", wantName: "README.md"},
		{name: "head default", wantName: "README.md"},
		{name: "commit hash", ref: commit.String(), wantName: "docs"},
		{name: "nested", ref: "main", path: "/docs/", wantPath: "docs", wantName: "guide.md"},
		{name: "missing ref", ref: "missing", wantErr: true},
		{name: "missing path", ref: "main", path: "missing", wantErr: true},
		{name: "file path", ref: "main", path: "README.md", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := readTreeAt(store, test.ref, test.path)
			if test.wantErr {
				if !errors.Is(err, meta.ErrNotFound) {
					t.Fatalf("error %v", err)
				}
				return
			}
			if err != nil || result.Path != test.wantPath || result.Commit != commit.String() {
				t.Fatalf("result %+v, error %v", result, err)
			}
			found := false
			for _, entry := range result.Entries {
				found = found || entry.Name == test.wantName
			}
			if !found {
				t.Fatalf("missing entry %q in %+v", test.wantName, result.Entries)
			}
		})
	}
}

func TestTreeAtRoute(t *testing.T) {
	store, commit := seedNestedContent(t)
	handler := treeAPI(store)
	base := acctBase + "/namespaces/default/repos/app/tree"

	tests := []struct {
		name string
		path string
		want int
		body []string
	}{
		{name: "root", path: "?ref=main", want: http.StatusOK, body: []string{commit.String(), `"path":""`, `"name":"docs"`}},
		{name: "nested", path: "?ref=main&path=docs", want: http.StatusOK, body: []string{`"path":"docs"`, `"name":"guide.md"`}},
		{name: "trim slashes", path: "?ref=main&path=%2Fdocs%2F", want: http.StatusOK, body: []string{`"path":"docs"`}},
		{name: "missing ref", path: "?ref=missing", want: http.StatusNotFound},
		{name: "missing path", path: "?ref=main&path=nope", want: http.StatusNotFound},
		{name: "file is not directory", path: "?ref=main&path=README.md", want: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := doJSON(t, handler, http.MethodGet, base+test.path, "", nil)
			if rec.Code != test.want {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			for _, fragment := range test.body {
				if !strings.Contains(rec.Body.String(), fragment) {
					t.Fatalf("missing %q in %s", fragment, rec.Body.String())
				}
			}
		})
	}
	root, err := readTreeAt(store, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	hashRoute := doJSON(t, handler, http.MethodGet, base+"/"+root.Tree, "", nil)
	if hashRoute.Code != http.StatusOK || !strings.Contains(hashRoute.Body.String(), `"name":"README.md"`) {
		t.Fatalf("hash tree route %d: %s", hashRoute.Code, hashRoute.Body.String())
	}

	escaped := base + "?" + url.Values{"ref": {commit.String()}, "path": {"docs"}}.Encode()
	if rec := doJSON(t, handler, http.MethodGet, escaped, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("commit ref status %d: %s", rec.Code, rec.Body.String())
	}
}

type treeMeta struct{ meta.Store }

func (treeMeta) Namespaces() meta.Namespaces { return treeNamespaces{} }
func (treeMeta) Repos() meta.Repos           { return treeRepos{} }
func (treeMeta) RepoTokens() meta.RepoTokens { return nil }

type treeNamespaces struct{ meta.Namespaces }

func (treeNamespaces) GetByName(_ context.Context, account types.AccountID, name types.NamespaceName) (types.Namespace, error) {
	if account != "local" || name != "default" {
		return types.Namespace{}, meta.ErrNotFound
	}
	return types.Namespace{ID: "namespace_1", AccountID: account, Name: name}, nil
}

type treeRepos struct{ meta.Repos }

func (treeRepos) GetByName(_ context.Context, namespace types.NamespaceID, name types.RepoName) (types.Repo, error) {
	if namespace != "namespace_1" || name != "app" {
		return types.Repo{}, meta.ErrNotFound
	}
	return types.Repo{ID: "repo_1", NamespaceID: namespace, Name: name, DefaultBranch: "main", Status: types.RepoReady}, nil
}

func treeAPI(store storer.Storer) http.Handler {
	services := service.New(treeMeta{}, nil, "http://example.test")
	cfg := config.Config{Auth: config.Auth{Mode: "none"}}
	return NewWithDependencies(services, cfg, Dependencies{ReadGit: fixedReader(store)})
}

func seedNestedContent(t *testing.T) (storer.Storer, plumbing.Hash) {
	t.Helper()
	store, err := gitstore.Open(objecttest.NewMem(), &contentRefs{data: map[string]string{}}, "local", "repo_1")
	if err != nil {
		t.Fatal(err)
	}
	readme := putBlobObj(t, store, "readme")
	guide := putBlobObj(t, store, "guide")
	docs := putTreeEntries(t, store, []object.TreeEntry{{Name: "guide.md", Mode: filemode.Regular, Hash: guide}})
	root := putTreeEntries(t, store, []object.TreeEntry{
		{Name: "README.md", Mode: filemode.Regular, Hash: readme},
		{Name: "docs", Mode: filemode.Dir, Hash: docs},
	})
	commit := putCommitObj(t, store, root)
	if err := store.SetReference(plumbing.NewHashReference("refs/heads/main", commit)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetReference(plumbing.NewSymbolicReference("HEAD", "refs/heads/main")); err != nil {
		t.Fatal(err)
	}
	return store, commit
}

func putTreeEntries(t *testing.T, store storer.Storer, entries []object.TreeEntry) plumbing.Hash {
	t.Helper()
	encoded := store.NewEncodedObject()
	if err := (&object.Tree{Entries: entries}).Encode(encoded); err != nil {
		t.Fatal(err)
	}
	hash, err := store.SetEncodedObject(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
