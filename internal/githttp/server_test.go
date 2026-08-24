package githttp

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

type staticTokens struct {
	scope types.Scope
	err   error
}

func (s staticTokens) Lookup(context.Context, string, string, string) (types.Scope, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.scope, nil
}

type keyedRefs struct {
	mu   sync.Mutex
	data map[string]string
}

func (k *keyedRefs) key(repo types.RepoID, name string) string {
	return string(repo) + "|" + name
}

func (k *keyedRefs) Get(_ context.Context, repo types.RepoID, name string) (types.Ref, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	sha, ok := k.data[k.key(repo, name)]
	if !ok {
		return types.Ref{}, meta.ErrNotFound
	}
	return types.Ref{RepoID: repo, Name: name, SHA: sha}, nil
}

func (k *keyedRefs) List(_ context.Context, repo types.RepoID) ([]types.Ref, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []types.Ref
	pref := string(repo) + "|"
	for key, sha := range k.data {
		if strings.HasPrefix(key, pref) {
			out = append(out, types.Ref{RepoID: repo, Name: strings.TrimPrefix(key, pref), SHA: sha})
		}
	}
	return out, nil
}

func (k *keyedRefs) CompareAndSwap(_ context.Context, repo types.RepoID, name, oldSHA, newSHA string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	key := k.key(repo, name)
	cur, ok := k.data[key]
	if oldSHA == "" {
		if ok {
			return meta.ErrCASConflict
		}
		if newSHA != "" {
			k.data[key] = newSHA
		}
		return nil
	}
	if !ok || cur != oldSHA {
		return meta.ErrCASConflict
	}
	if newSHA == "" {
		delete(k.data, key)
		return nil
	}
	k.data[key] = newSHA
	return nil
}

func (k *keyedRefs) DeleteAll(_ context.Context, repo types.RepoID) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	pref := string(repo) + "|"
	for key := range k.data {
		if strings.HasPrefix(key, pref) {
			delete(k.data, key)
		}
	}
	return nil
}

func testGitServer(t *testing.T, tokens TokenLookup) *httptest.Server {
	t.Helper()
	objs := objecttest.NewMem()
	refs := &keyedRefs{data: map[string]string{}}
	open := func(ns, repo string) (storer.Storer, error) {
		st, err := gitstore.Open(objs, refs, "local", types.RepoID(ns+"-"+repo))
		if err != nil {
			return nil, err
		}
		_ = st.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main"))
		return st, nil
	}
	return httptest.NewServer(New(open, tokens))
}

func TestUnauthorizedAndForbidden(t *testing.T) {
	t.Parallel()
	srv := testGitServer(t, staticTokens{err: errors.New("nope")})
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/git/default/app.git/info/refs?service=git-upload-pack", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("code %d", resp.StatusCode)
	}
	srv2 := testGitServer(t, staticTokens{scope: types.ScopeRead})
	t.Cleanup(srv2.Close)
	req, _ = http.NewRequest(http.MethodGet, srv2.URL+"/git/default/app.git/info/refs?service=git-receive-pack", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("code %d", resp.StatusCode)
	}
}

func TestGitPushCloneFetch(t *testing.T) {
	testkit.GitAvailable(t)
	srv := testGitServer(t, staticTokens{scope: types.ScopeWrite})
	t.Cleanup(srv.Close)
	u := strings.TrimPrefix(srv.URL, "http://")
	remote := "http://x:write@" + u + "/git/default/app.git"
	src := testkit.TempRepo(t)
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hooks := t.TempDir()
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "add", "README.md")
	testkit.RunGit(t, src, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false", "commit", "-m", "init")
	testkit.RunGit(t, src, "remote", "add", "origin", remote)
	testkit.RunGit(t, src, "-c", "protocol.version=1", "push", "-u", "origin", "main")

	dst := t.TempDir()
	testkit.RunGit(t, t.TempDir(), "-c", "protocol.version=1", "clone", remote, dst)
	if _, err := os.Stat(filepath.Join(dst, "README.md")); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, dst, "-c", "protocol.version=1", "fetch", "origin")
}

func TestBearerOrBasic(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer abc")
	if bearerOrBasic(r) != "abc" {
		t.Fatal("bearer")
	}
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("x:secret")))
	if bearerOrBasic(r) != "secret" {
		t.Fatal("basic")
	}
}
