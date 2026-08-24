package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
)

func TestForkImportHTTP(t *testing.T) {
	h := testAPI(t, "none", "")
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "src"})
	// Jobs unset → 500
	rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos/src/fork", "", map[string]string{"name": "dst"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("no jobs %d", rec.Code)
	}
	Jobs = jobs.New(nil, objecttest.NewMem(), "http://x")
	t.Cleanup(func() { Jobs = nil })
	req := httptest.NewRequest(http.MethodPost, acctBase+"/namespaces/default/repos/src/fork", bytes.NewReader([]byte("{")))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad fork json %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, acctBase+"/namespaces/default/repos/src/import", bytes.NewReader([]byte("{")))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad import json %d", rec.Code)
	}
}
