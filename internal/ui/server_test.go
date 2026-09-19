package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/types"
)

const fixtureRepoAPI = "/client/v4/accounts/local/artifacts/namespaces/default/repos/app"

type fixtureREST struct {
	fail  bool
	empty bool
}

func (f fixtureREST) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.fail {
		writeFixtureError(w, http.StatusServiceUnavailable, "REST unavailable")
		return
	}
	switch r.URL.Path {
	case "/client/v4/accounts/local/artifacts/namespaces":
		writeFixture(w, []types.Namespace{{Name: "safe<script>"}})
	case "/client/v4/accounts/local/artifacts/namespaces/default/repos":
		writeFixture(w, []types.Repo{{Name: "app", Description: "agent artifacts"}})
	case fixtureRepoAPI:
		writeFixture(w, types.Repo{Name: "app", Description: "<script>repo</script>", DefaultBranch: "main",
			Remote: "http://local/git/local/default/app.git", WALSequence: 1})
	case fixtureRepoAPI + "/refs":
		if f.empty {
			writeFixture(w, []types.Ref{})
			return
		}
		writeFixture(w, []types.Ref{{Name: "refs/heads/main", SHA: strings.Repeat("a", 40)},
			{Name: "refs/heads/feature", SHA: strings.Repeat("b", 40)}, {Name: "refs/tags/v1", SHA: strings.Repeat("c", 40)}})
	case fixtureRepoAPI + "/tree":
		f.tree(w, r)
	case fixtureRepoAPI + "/file":
		f.file(w, r)
	case fixtureRepoAPI + "/log":
		message := "initial"
		if r.URL.Query().Get("ref") == "feature" {
			message = "feature commit"
		}
		writeFixture(w, []types.LogEntry{{Hash: strings.Repeat("d", 40), Message: message,
			Author: types.Signature{Name: "Agent", When: time.Unix(1_700_000_000, 0).UTC()}}})
	case fixtureRepoAPI + "/wal":
		writeFixture(w, []types.PackWAL{{Sequence: 1, Checksum: strings.Repeat("e", 64), Size: 42}})
	default:
		writeFixtureError(w, http.StatusNotFound, "missing fixture")
	}
}

func (f fixtureREST) tree(w http.ResponseWriter, r *http.Request) {
	if f.empty {
		writeFixtureError(w, http.StatusNotFound, "not found")
		return
	}
	path := r.URL.Query().Get("path")
	var entries []types.TreeEntry
	switch {
	case r.URL.Query().Get("ref") == "feature":
		entries = []types.TreeEntry{{Name: "feature.txt", Type: "blob", Mode: "0100644"}}
	case path == "docs":
		entries = []types.TreeEntry{{Name: "guide.md", Type: "blob", Mode: "0100644"}}
	default:
		entries = []types.TreeEntry{
			{Name: "README.md", Type: "blob", Mode: "0100644"},
			{Name: "docs", Type: "tree", Mode: "0040000"},
			{Name: "image.bin", Type: "blob", Mode: "0100644"},
			{Name: "large.txt", Type: "blob", Mode: "0100644"},
		}
	}
	writeFixture(w, treeResponse{Ref: r.URL.Query().Get("ref"), Commit: strings.Repeat("f", 40),
		Tree: strings.Repeat("a", 40), Path: path, Entries: entries})
}

func (fixtureREST) file(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("path") {
	case "README.md":
		_, _ = w.Write([]byte("<script>alert(1)</script>\n"))
	case "docs/guide.md":
		_, _ = w.Write([]byte("# Guide\n"))
	case "image.bin":
		_, _ = w.Write([]byte{0, 1, 2, 3})
	case "large.txt":
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxPreviewBytes+1))
	default:
		writeFixtureError(w, http.StatusNotFound, "file not found")
	}
}

func writeFixture(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": result, "success": true, "errors": []any{}, "messages": []any{},
	})
}

func writeFixtureError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": nil, "success": false, "errors": []map[string]any{{"code": 1, "message": message}}, "messages": []any{},
	})
}

func newBrowser(t *testing.T, rest http.Handler) http.Handler {
	t.Helper()
	handler, err := New(rest)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func browserGet(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func requireFragments(t *testing.T, recorder *httptest.ResponseRecorder, fragments ...string) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	for _, fragment := range fragments {
		if !strings.Contains(recorder.Body.String(), fragment) {
			t.Fatalf("missing %q in %q", fragment, recorder.Body.String())
		}
	}
}

func TestBrowserRepositoryNavigation(t *testing.T) {
	handler := newBrowser(t, fixtureREST{})
	tests := []struct {
		path      string
		fragments []string
	}{
		{path: "/", fragments: []string{"Local artifact repositories", "REST API"}},
		{path: "/local", fragments: []string{"Namespaces", "safe&lt;script&gt;"}},
		{path: "/local/default", fragments: []string{"agent artifacts"}},
		{path: "/local/default/app", fragments: []string{"README.md", "docs", "Commit", "ffffffffffff"}},
		{path: "/local/default/app/browse/docs?ref=main", fragments: []string{"guide.md", "docs", ".."}},
		{path: "/local/default/app?ref=feature", fragments: []string{"feature.txt", `value="feature" selected`}},
		{path: "/local/default/app/commits", fragments: []string{"initial", "Agent"}},
		{path: "/local/default/app/commits?ref=feature", fragments: []string{"feature commit"}},
		{path: "/local/default/app/wal", fragments: []string{"Storage diagnostic", "42", strings.Repeat("e", 64)}},
		{path: "/local/default/app/settings", fragments: []string{"Default branch", "Read only", "WAL sequence"}},
		{path: "/assets/style.css", fragments: []string{"file-content"}},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := browserGet(handler, test.path)
			requireFragments(t, recorder, test.fragments...)
			if strings.Contains(recorder.Body.String(), "<script>repo") {
				t.Fatal("repository description was not escaped")
			}
			if strings.Contains(recorder.Body.String(), "Storage version") {
				t.Fatal("repository settings expose an internal storage revision")
			}
		})
	}
}

func TestBrowserFilePreviewAndDownload(t *testing.T) {
	handler := newBrowser(t, fixtureREST{})
	text := browserGet(handler, "/local/default/app/file/README.md?ref=main")
	requireFragments(t, text, "README.md", "&lt;script&gt;alert(1)&lt;/script&gt;", "Download")
	if strings.Contains(text.Body.String(), "<script>alert") {
		t.Fatal("stored HTML executed in preview markup")
	}
	requireFragments(t, browserGet(handler, "/local/default/app/file/image.bin?ref=main"), "Binary content is not rendered")
	requireFragments(t, browserGet(handler, "/local/default/app/file/large.txt?ref=main"), "256 KiB preview limit")
	requireFragments(t, browserGet(handler, "/local/default/app/file/missing?ref=main"), "file not found")

	download := browserGet(handler, "/local/default/app/download/image.bin?ref=main")
	if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), []byte{0, 1, 2, 3}) {
		t.Fatalf("download %d %v", download.Code, download.Body.Bytes())
	}
	if download.Header().Get("Content-Disposition") != "attachment" || download.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe download headers: %v", download.Header())
	}
}

func TestPreviewContentTypes(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		want        bool
	}{
		{name: "plain text", body: []byte("hello"), contentType: "text/plain; charset=utf-8", want: true},
		{name: "json", body: []byte(`{"ok":true}`), contentType: "application/json", want: true},
		{name: "svg source", body: []byte("<svg></svg>"), contentType: "image/svg+xml", want: true},
		{name: "binary type", body: []byte("PK-data"), contentType: "application/zip"},
		{name: "nul byte", body: []byte{'a', 0}, contentType: "text/plain"},
		{name: "invalid utf8", body: []byte{0xff}, contentType: "text/plain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canPreview(test.body, test.contentType); got != test.want {
				t.Fatalf("canPreview = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBrowserRESTFailuresAndEmptyRepo(t *testing.T) {
	failed := newBrowser(t, fixtureREST{fail: true})
	for _, path := range []string{"/local", "/local/default", "/local/default/app", "/local/default/app/commits",
		"/local/default/app/wal", "/local/default/app/settings", "/local/default/app/file/README.md"} {
		requireFragments(t, browserGet(failed, path), "REST unavailable")
	}
	empty := newBrowser(t, fixtureREST{empty: true})
	recorder := browserGet(empty, "/local/default/app")
	requireFragments(t, recorder, "Repository or directory is empty")
	if strings.Contains(recorder.Body.String(), "class=\"error\"") {
		t.Fatal("empty repository rendered as an error")
	}
}

func TestBrowserConstructionAndMalformedREST(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("accepted nil REST handler")
	}
	malformed := newBrowser(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	requireFragments(t, browserGet(malformed, "/local"), "decode REST response")
	if csp := browserGet(newBrowser(t, fixtureREST{}), "/").Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src &#39;none&#39;") && !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("missing CSP: %q", csp)
	}
}

func TestBrowserIsolatesNestedRESTRouteContext(t *testing.T) {
	rest := chi.NewRouter()
	rest.Handle("/*", fixtureREST{})
	handler := newBrowser(t, rest)
	recorder := browserGet(handler, "/local/default/app")
	requireFragments(t, recorder, "README.md", "docs", "Commit")
	if strings.Contains(recorder.Body.String(), "class=\"error\"") {
		t.Fatalf("nested REST routing failed: %s", recorder.Body.String())
	}
}
