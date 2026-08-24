package api

import (
	"net/http"
	"testing"
)

func TestNamespaces(t *testing.T) {
	h := testAPI(t, "none", "")
	rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces", "", map[string]string{"namespace": "default"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPost, acctBase+"/namespaces", "", map[string]string{"namespace": "default"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodPost, acctBase+"/namespaces", "", map[string]string{"namespace": "-bad"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad name %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces/missing", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces?limit=10", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d", rec.Code)
	}
}
