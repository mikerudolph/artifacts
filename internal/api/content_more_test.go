package api

import (
	"net/http"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/storer"
)

func TestContentErrors(t *testing.T) {
	h := testAPI(t, "none", "")
	base := acctBase + "/namespaces/default/repos/app"
	if rec := doJSON(t, h, http.MethodGet, base+"/log", "", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("no git %d", rec.Code)
	}
	doJSON(t, h, http.MethodPost, acctBase+"/namespaces/default/repos", "", map[string]string{"name": "app"})
	st, _, _, _ := seedContent(t)
	OpenGit = func(string, string, string) (storer.Storer, error) { return st, nil }
	t.Cleanup(func() { OpenGit = nil })
	if rec := doJSON(t, h, http.MethodGet, base+"/commit/"+"0000000000000000000000000000000000000000", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("commit %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, base+"/tree/"+"0000000000000000000000000000000000000000", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("tree %d", rec.Code)
	}
	if rec := rawGet(h, base+"/blob/"+"0000000000000000000000000000000000000000"); rec.Code != http.StatusNotFound {
		t.Fatalf("blob %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, base+"/log?ref=missing", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("log ref %d", rec.Code)
	}
	if rec := rawGet(h, base+"/raw/main/nope.txt"); rec.Code != http.StatusNotFound {
		t.Fatalf("raw %d", rec.Code)
	}
}
