package api

import (
	"net/http"
	"testing"
)

func TestRepos(t *testing.T) {
	h := testAPI(t, "none", "")
	rec := doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Remote string `json:"remote"`
		Token  string `json:"token"`
	}
	decodeResult(t, rec, &created)
	if created.Remote == "" || created.Token == "" {
		t.Fatalf("%+v", created)
	}
	rec = doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos/app", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos?limit=10&sort=name&direction=asc&search=ap", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodDelete, acctBase+"/namespaces/default/repos/app", "", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("delete %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces/default/repos/nope", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing %d", rec.Code)
	}
}
