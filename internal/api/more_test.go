package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
)

func TestMoreAPIErrors(t *testing.T) {
	h := testAPI(t, "none", "")
	if rec := doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos?sort=nope", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("sort %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos?direction=side", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("dir %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/tokens", "", map[string]any{"repo": "nope"}); rec.Code != http.StatusNotFound {
		t.Fatalf("tok missing %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos/nope/tokens?state=bogus", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("state %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodDelete, acctBase+"/namespaces/default/tokens/missing", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("revoke %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces", "", map[string]string{"namespace": "ok", "jurisdiction": "apac"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("jurisdiction %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, acctBase+"/namespaces/default/repos", bytes.NewReader([]byte("{")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("repo json %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, acctBase+"/namespaces/default/tokens", bytes.NewReader([]byte("{")))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("token json %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodDelete, acctBase+"/namespaces/default/repos/missing", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("del %d", rec.Code)
	}
}

func TestForkWithJobs(t *testing.T) {
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	st, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, time.Now, "http://example.test")
	h := New(svc, config.Config{Auth: config.Auth{Mode: "none"}})
	Jobs = jobs.New(st, objecttest.NewMem(), "http://example.test")
	t.Cleanup(func() { Jobs = nil })
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "src"})
	rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos/src/fork", "", map[string]string{"name": "dst"})
	if rec.Code != http.StatusOK {
		t.Fatalf("fork %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos/src/import", "", map[string]string{"url": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("import %d", rec.Code)
	}
}
