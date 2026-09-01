package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestArtifactsCreatesRepository(t *testing.T) {
	t.Parallel()
	var authorized bool
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		authorized = authorized || r.Header.Get("Authorization") == "Bearer control"
		if r.Method == http.MethodGet {
			return testEnvelope(http.StatusNotFound, false, nil), nil
		}
		return testEnvelope(http.StatusOK, true, map[string]string{
			"remote": "https://example.test/brain.git", "token": "repo-token",
		}), nil
	})
	client := newArtifactsClient(configuration{Root: "https://example.test", Account: "local", Token: "control"})
	client.client.Transport = transport
	access, err := client.ensureRepository(context.Background(), "memory", "brain")
	if err != nil || access.Remote == "" || access.Credential != "repo-token" || !authorized {
		t.Fatalf("access=%+v auth=%v error=%v", access, authorized, err)
	}
}

func TestArtifactsUsesExistingRepository(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return testEnvelope(http.StatusOK, true, map[string]string{"remote": "https://example.test/brain.git"}), nil
		}
		return testEnvelope(http.StatusOK, true, map[string]string{"plaintext": "fresh-token"}), nil
	})
	client := newArtifactsClient(configuration{Root: "https://example.test", Account: "local"})
	client.client.Transport = transport
	access, err := client.ensureRepository(context.Background(), "memory", "brain")
	if err != nil || access.Credential != "fresh-token" {
		t.Fatalf("access=%+v error=%v", access, err)
	}
}

func TestArtifactsBootstrapConflictAndMalformedAccess(t *testing.T) {
	t.Parallel()
	gets := 0
	client := newArtifactsClient(configuration{Root: "https://example.test", Account: "local"})
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			gets++
			if gets == 1 {
				return testEnvelope(http.StatusNotFound, false, nil), nil
			}
			return testEnvelope(http.StatusOK, true, map[string]string{"remote": "https://example.test/brain.git"}), nil
		}
		if strings.HasSuffix(r.URL.Path, "/repos") {
			return testEnvelope(http.StatusConflict, false, nil), nil
		}
		return testEnvelope(http.StatusOK, true, map[string]string{"plaintext": "token"}), nil
	})
	access, err := client.ensureRepository(context.Background(), "memory", "brain")
	if err != nil || access.Credential != "token" || gets != 2 {
		t.Fatalf("conflict access=%+v gets=%d error=%v", access, gets, err)
	}
	for _, result := range []any{map[string]string{"remote": "only"}, map[string]string{"token": "only"}} {
		bad := newArtifactsClient(configuration{Root: "https://example.test", Account: "local"})
		bad.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testEnvelope(http.StatusOK, true, result), nil
		})
		if _, err := bad.ensureRepository(context.Background(), "memory", "brain"); err == nil {
			t.Fatalf("malformed access accepted: %v", result)
		}
	}
}

func TestArtifactsClientErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"success":false}`},
		{"server error", http.StatusInternalServerError, `{"success":false}`},
		{"malformed", http.StatusOK, `not-json`},
		{"bad result", http.StatusOK, `{"success":true,"result":"wrong"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newArtifactsClient(configuration{Root: "https://example.test", Account: "local"})
			client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})
			var output repositoryResult
			if err := client.call(context.Background(), http.MethodGet, "/x", nil, &output); err == nil {
				t.Fatal("error response accepted")
			}
		})
	}
}

func TestArtifactsUnreachableAndEncodingErrors(t *testing.T) {
	t.Parallel()
	client := &artifactsClient{base: "http://127.0.0.1:1", client: &http.Client{Timeout: 50 * time.Millisecond}}
	err := client.call(context.Background(), http.MethodGet, "/x", nil, nil)
	var apiErr *artifactsError
	if !errors.As(err, &apiErr) || apiErr.Status != 0 || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("unreachable error=%v", err)
	}
	client.base = "://bad"
	if err := client.call(context.Background(), http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("invalid request URL accepted")
	}
	if err := client.call(context.Background(), http.MethodPost, "/x", make(chan int), nil); err == nil {
		t.Fatal("unencodable request accepted")
	}
	if (&artifactsError{Status: 401, Operation: "get"}).Error() != "Artifacts get failed with status 401" ||
		(&artifactsError{Malformed: true}).Error() != "Artifacts returned a malformed response" {
		t.Fatal("unexpected safe error text")
	}
}

func TestConfiguration(t *testing.T) {
	t.Setenv("ARTIFACTS_URL", " https://example.test/ ")
	t.Setenv("ARTIFACTS_ACCOUNT", "person")
	t.Setenv("ARTIFACTS_API_TOKEN", "token")
	t.Setenv("ARTIFACTS_MEMORY_NAMESPACE", "memories")
	t.Setenv("ARTIFACTS_MEMORY_REPO", "brain")
	cfg, err := loadConfiguration()
	if err != nil || cfg.Root != "https://example.test" || cfg.Account != "person" || cfg.Token != "token" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
	for _, value := range []string{"not-a-url", "/relative"} {
		t.Setenv("ARTIFACTS_URL", value)
		if _, err := loadConfiguration(); err == nil {
			t.Fatalf("invalid URL accepted: %s", value)
		}
	}
	t.Setenv("ARTIFACTS_URL", "https://example.test")
	t.Setenv("ARTIFACTS_ACCOUNT", " ")
	if cfg, err := loadConfiguration(); err != nil || cfg.Account != "local" {
		t.Fatalf("empty environment did not use default: %+v %v", cfg, err)
	}
	if repoAPIPath("team space", "my brain") != "/namespaces/team%20space/repos/my%20brain" {
		t.Fatal("repository path was not escaped")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testEnvelope(status int, success bool, result any) *http.Response {
	data, _ := json.Marshal(map[string]any{"success": success, "result": result, "errors": []any{}})
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}
}
