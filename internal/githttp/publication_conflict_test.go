package githttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type failedPublication struct {
	routeBackend
	err error
}

func (b failedPublication) Receive(context.Context, types.Repo, io.Reader, string) ([]byte, error) {
	return nil, b.err
}

func TestPublicationConflictStatus(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{meta.ErrCASConflict, http.StatusConflict},
		{errors.New("storage unavailable"), http.StatusInternalServerError},
	} {
		h := NewRepository(failedPublication{err: tc.err}, routeLookup{repo: types.Repo{ID: "repo", Status: types.RepoReady}}, routeAuth{}, false)
		req := httptest.NewRequest(http.MethodPost, "/git/acct/ns/repo.git/git-receive-pack", nil)
		req.Header.Set("Authorization", "Bearer test")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("got %d want %d", res.Code, tc.status)
		}
	}
}
