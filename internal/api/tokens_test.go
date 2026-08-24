package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokens(t *testing.T) {
	h := testAPI(t, "none", "")
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/tokens", "", map[string]any{
		"repo": "app", "scope": "read", "ttl": 3600,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var tok struct {
		ID        string `json:"id"`
		Plaintext string `json:"plaintext"`
	}
	decodeResult(t, rec, &tok)
	rec = doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/tokens", "", map[string]any{
		"repo": "app", "ttl": 1,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ttl %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos/app/tokens?state=active&per_page=10&page=1", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodDelete, acctBase+"/namespaces/default/tokens/"+tok.ID, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke %d %s", rec.Code, rec.Body.String())
	}
}

func TestBadJSON(t *testing.T) {
	h := testAPI(t, "none", "")
	req := httptest.NewRequest(http.MethodPost, acctBase+"/namespaces", bytes.NewReader([]byte("{")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json %d", rec.Code)
	}
}
