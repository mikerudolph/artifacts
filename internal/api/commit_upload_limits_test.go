package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestMultipartAdmissionAndRelease(t *testing.T) {
	h := repositoryAPIConfigured(t, repositoryContent{}, config.Config{Auth: config.Auth{Mode: "none"}, Uploads: config.Uploads{Concurrent: 1, MaxBytes: 5}})
	collection := acctBase + "/namespaces/limits/repos"
	expectStatus(t, doJSON(t, h, "POST", collection, "", map[string]string{"name": "limits"}), 200)
	base := collection + "/limits"
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan *httptest.ResponseRecorder, 1)
	var once sync.Once
	defer once.Do(func() { close(release) })
	req := uploadRequest(t, base, "", "", `{"files":[{"path":"x","part":"x"}]}`, "x", "12345")
	req.Body = &hookBody{ReadCloser: req.Body, hook: func() { close(entered); <-release }}
	go func() { done <- serveUpload(h, req) }()
	<-entered
	blocked := serveUpload(h, uploadRequest(t, base, "", "", `{"replace":true}`))
	expectStatus(t, blocked, 503)
	if blocked.Header().Get("Retry-After") == "" {
		t.Fatal("missing retry guidance")
	}
	once.Do(func() { close(release) })
	expectStatus(t, <-done, 201)
	expectStatus(t, serveUpload(h, uploadRequest(t, base, "", "", `{"files":[{"path":"x","part":"x"}]}`, "x", "123456")), 413)
	expectStatus(t, serveUpload(h, uploadRequest(t, base, "", "", `{"replace":true}`)), 201)
}

func TestMultipartDeadlinesInterruptBody(t *testing.T) {
	for _, total := range []bool{false, true} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			idle, deadline := 40*time.Millisecond, time.Second
			if total {
				idle, deadline = time.Second, 40*time.Millisecond
			}
			cfg := config.Config{Auth: config.Auth{Mode: "none"}, HTTP: config.HTTP{StreamIdleTimeout: idle}, Uploads: config.Uploads{Timeout: deadline}}
			h := repositoryAPIConfigured(t, repositoryContent{}, cfg)
			collection := acctBase + "/namespaces/limits/repos"
			expectStatus(t, doJSON(t, h, "POST", collection, "", map[string]string{"name": "slow"}), 200)
			server := httptest.NewServer(h)
			defer server.Close()
			conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			_, err = fmt.Fprintf(conn, "POST %s/slow/commits HTTP/1.1\r\nHost: localhost\r\nContent-Type: multipart/form-data; boundary=x\r\nContent-Length: 1000\r\n\r\n--x\r\n", collection)
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != 408 && response.StatusCode != 503 {
				t.Fatalf("deadline status: %d", response.StatusCode)
			}
		})
	}
}

func TestMultipartScratchFailureAndErrorRecovery(t *testing.T) {
	h := repositoryAPIConfigured(t, repositoryContent{}, config.Config{Auth: config.Auth{Mode: "none"}})
	collection := acctBase + "/namespaces/limits/repos"
	expectStatus(t, doJSON(t, h, "POST", collection, "", map[string]string{"name": "scratch"}), 200)
	base := collection + "/scratch"
	t.Setenv("TMPDIR", t.TempDir()+"/missing")
	expectStatus(t, serveUpload(h, uploadRequest(t, base, "", "", `{"files":[{"path":"x","part":"x"}]}`, "x", "x")), 500)
	for _, err := range []error{syscall.ENOSPC, syscall.EDQUOT, context.DeadlineExceeded, context.Canceled} {
		w := httptest.NewRecorder()
		writeUploadError(w, &os.PathError{Op: "write", Path: "private", Err: err})
		expectStatus(t, w, 503)
		if strings.Contains(w.Body.String(), "private") {
			t.Fatal("scratch path exposed")
		}
	}
	w := httptest.NewRecorder()
	writeUploadError(w, io.ErrUnexpectedEOF)
	expectStatus(t, w, 400)
}

type delayedCommit struct{ repositoryContent }

func (d delayedCommit) Commit(ctx context.Context, repo types.Repo, input types.CommitInput) (types.CommitResult, error) {
	select {
	case <-ctx.Done():
		return types.CommitResult{}, ctx.Err()
	case <-time.After(300 * time.Millisecond):
		return d.repositoryContent.Commit(ctx, repo, input)
	}
}

func TestMultipartFinalizationOutlivesReadIdle(t *testing.T) {
	cfg := config.Config{Auth: config.Auth{Mode: "none"}, HTTP: config.HTTP{StreamIdleTimeout: 40 * time.Millisecond}}
	h := repositoryAPIConfigured(t, delayedCommit{}, cfg)
	collection := acctBase + "/namespaces/limits/repos"
	expectStatus(t, doJSON(t, h, "POST", collection, "", map[string]string{"name": "finalize"}), 200)
	server := httptest.NewServer(h)
	defer server.Close()
	input := uploadRequest(t, collection+"/finalize", "", "", `{"files":[{"path":"x","part":"x"}]}`, "x", "x")
	req, err := http.NewRequest("POST", server.URL+input.URL.Path, input.Body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = input.Header
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 201 {
		t.Fatalf("finalization interrupted: HTTP %d", response.StatusCode)
	}
}
