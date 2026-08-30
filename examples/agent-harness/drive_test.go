package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type failureTransport struct {
	mu           sync.Mutex
	repositories map[string]bool
}

func (f *failureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := request.URL.Path
	if strings.HasSuffix(path, "/namespaces") {
		return jsonResponse(http.StatusOK, []any{}), nil
	}
	if strings.HasSuffix(path, "/commits") {
		return failureResponse(http.StatusInternalServerError), nil
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch request.Method {
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(request.Body).Decode(&input)
		key := parts[len(parts)-2] + "/" + input.Name
		f.repositories[key] = true
		return jsonResponse(http.StatusOK, map[string]any{
			"remote": "http://example.test/git/local/" + key + ".git", "token": "repo-secret",
		}), nil
	case http.MethodDelete:
		key := parts[len(parts)-3] + "/" + parts[len(parts)-1]
		delete(f.repositories, key)
		return jsonResponse(http.StatusAccepted, map[string]string{"id": "job"}), nil
	case http.MethodGet:
		key := parts[len(parts)-3] + "/" + parts[len(parts)-1]
		if !f.repositories[key] {
			return failureResponse(http.StatusNotFound), nil
		}
		return jsonResponse(http.StatusOK, map[string]string{"name": parts[len(parts)-1]}), nil
	default:
		return failureResponse(http.StatusMethodNotAllowed), nil
	}
}

func (f *failureTransport) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.repositories)
}

func TestCoreFailureCleansFinalServerState(t *testing.T) {
	t.Parallel()
	transport := &failureTransport{repositories: map[string]bool{}}
	h := newHarness(configuration{root: "http://example.test", account: "local", token: "control"})
	h.client.Transport = transport
	dir := t.TempDir()
	report, err := verifyCore(context.Background(), h, dir)
	if err == nil || report.Classification != classProduct {
		t.Fatalf("verifyCore error=%v report=%+v", err, report)
	}
	if remaining := transport.count(); remaining != 0 {
		t.Fatalf("final server state retained %d repositories", remaining)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "report.json")); statErr != nil {
		t.Fatalf("evidence did not survive cleanup: %v", statErr)
	}
	if len(report.Assertions) == 0 || !report.Assertions[len(report.Assertions)-1].Passed {
		t.Fatalf("cleanup final-state assertion missing: %+v", report.Assertions)
	}
}

func TestCoreEvidenceFailureIsClassified(t *testing.T) {
	t.Parallel()
	transport := &failureTransport{repositories: map[string]bool{}}
	h := newHarness(configuration{root: "http://example.test", account: "local", token: "control"})
	h.client.Transport = transport
	evidence := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(evidence, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := verifyCore(context.Background(), h, evidence)
	if err == nil || report.Classification != classEvidence {
		t.Fatalf("verifyCore error=%v report=%+v", err, report)
	}
	if remaining := transport.count(); remaining != 0 {
		t.Fatalf("final server state retained %d repositories", remaining)
	}
}

func jsonResponse(status int, result any) *http.Response {
	data, _ := json.Marshal(map[string]any{"success": true, "result": result, "errors": []any{}})
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}
}

func failureResponse(status int) *http.Response {
	data, _ := json.Marshal(map[string]any{
		"success": false, "result": nil, "errors": []map[string]string{{"message": "failed"}},
	})
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}
}
