package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

type repositoryContent struct{ err error }

func (r repositoryContent) Commit(context.Context, types.Repo, types.CommitInput) (types.CommitResult, error) {
	return types.CommitResult{SHA: "abc", Sequence: 1}, r.err
}
func (r repositoryContent) Refs(_ context.Context, repo types.Repo) ([]types.Ref, error) {
	return []types.Ref{{RepoID: repo.ID, Name: "refs/heads/main", SHA: "abc"}}, r.err
}
func (r repositoryContent) WAL(_ context.Context, repo types.Repo) ([]types.PackWAL, error) {
	return []types.PackWAL{{RepoID: repo.ID, Sequence: 1}}, r.err
}

func repositoryAPI(t *testing.T, content RepositoryContent) http.Handler {
	t.Helper()
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	metadata, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closer, ok := metadata.(interface{ Close() }); ok {
			closer.Close()
		}
	})
	services := service.New(metadata, time.Now, "http://example.test")
	return NewWithDependencies(services, config.Config{Auth: config.Auth{Mode: "none"}}, Dependencies{Repository: content})
}

func TestRepositoryPublicationRoutes(t *testing.T) {
	h := repositoryAPI(t, repositoryContent{})
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	base := acctBase + "/namespaces/default/repos/app"
	cases := []struct {
		method string
		path   string
		body   any
		want   int
	}{
		{http.MethodPost, base + "/commits", map[string]any{"message": "initial", "files": []map[string]string{{"path": "a", "content": "b"}}}, http.StatusCreated},
		{http.MethodGet, base + "/refs", nil, http.StatusOK},
		{http.MethodGet, base + "/wal", nil, http.StatusOK},
		{http.MethodPatch, base + "/settings", map[string]any{"description": "updated", "read_only": true}, http.StatusOK},
		{http.MethodGet, base + "/jobs", nil, http.StatusOK},
	}
	for _, tc := range cases {
		if rec := doJSON(t, h, tc.method, tc.path, "", tc.body); rec.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if rec := doJSON(t, h, http.MethodPatch, base+"/settings", "", map[string]string{"default_branch": "bad branch"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad settings %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, base+"/commits", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad commit %d", rec.Code)
	}
}

func TestRepositoryRouteDependencyAndErrors(t *testing.T) {
	h := repositoryAPI(t, nil)
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	base := acctBase + "/namespaces/default/repos/app"
	for _, path := range []string{base + "/commits", base + "/refs", base + "/wal"} {
		method := http.MethodGet
		if path == base+"/commits" {
			method = http.MethodPost
		}
		if rec := doJSON(t, h, method, path, "", map[string]any{"files": []any{}}); rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	h = repositoryAPI(t, repositoryContent{err: errors.New("boom")})
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	for _, path := range []string{base + "/commits", base + "/refs", base + "/wal"} {
		method := http.MethodGet
		if path == base+"/commits" {
			method = http.MethodPost
		}
		if rec := doJSON(t, h, method, path, "", map[string]any{"files": []map[string]string{{"path": "a", "content": "b"}}}); rec.Code != http.StatusInternalServerError {
			t.Fatalf("error %s: %d", path, rec.Code)
		}
	}
	missing := acctBase + "/namespaces/default/repos/missing"
	for _, path := range []string{missing + "/commits", missing + "/refs", missing + "/wal", missing + "/settings"} {
		method := http.MethodGet
		var body any
		if strings.HasSuffix(path, "/commits") {
			method = http.MethodPost
			body = map[string]any{"files": []map[string]string{{"path": "a", "content": "b"}}}
		}
		if strings.HasSuffix(path, "/settings") {
			method, body = http.MethodPatch, map[string]string{"description": "x"}
		}
		if rec := doJSON(t, h, method, path, "", body); rec.Code != http.StatusNotFound {
			t.Fatalf("missing %s: %d", path, rec.Code)
		}
	}
}
