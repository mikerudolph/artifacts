package s3

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

func TestEncryptionHeadersAndImmutableCopies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ mode, key string }{{"", ""}, {"AES256", ""}, {"aws:kms", ""}, {"aws:kms", "alias/artifacts"}} {
		t.Run(tc.mode+tc.key, func(t *testing.T) {
			t.Parallel()
			fixture := &encryptionEndpoint{t: t, mode: tc.mode, key: tc.key, objects: make(map[string][]byte), retry: true}
			server := httptest.NewServer(fixture)
			defer server.Close()
			st, err := New(t.Context(), config.S3{Endpoint: server.URL, Bucket: "bucket", Region: "us-east-1",
				AccessKey: "test-key", SecretKey: "test-secret", UsePathStyle: true, SSE: tc.mode, SSEKMSKeyID: tc.key})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := st.Put(t.Context(), "pack", strings.NewReader("content"), 7); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.Copy(t.Context(), "pack", "copy"); err != nil {
				t.Fatal(err)
			}
			reader, err := st.(object.RangeStore).GetRange(t.Context(), "copy", 1, 3)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil || string(body) != "ont" {
				t.Fatal("range read changed", err)
			}
			if err := st.Delete(t.Context(), "copy"); err != nil {
				t.Fatal(err)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.puts != 3 {
				t.Fatalf("expected original, retry, and copy upload; got %d", fixture.puts)
			}
		})
	}
}

type encryptionEndpoint struct {
	t       *testing.T
	mu      sync.Mutex
	mode    string
	key     string
	objects map[string][]byte
	puts    int
	retry   bool
}

func (s *encryptionEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mode, key := r.Header.Get("X-Amz-Server-Side-Encryption"), r.Header.Get("X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id")
	if r.Method == http.MethodPut {
		if mode != s.mode || key != s.key {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "<Error><Code>AccessDenied</Code></Error>")
			return
		}
		s.put(w, r)
		return
	}
	if mode != "" || key != "" {
		s.t.Error("encryption write headers appeared on a read or delete")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body, exists := s.objects[r.URL.Path]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	checksum := sha256.Sum256(body)
	w.Header().Set("X-Amz-Meta-Sha256", hex.EncodeToString(checksum[:]))
	if r.Method == http.MethodDelete {
		delete(s.objects, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	status := http.StatusOK
	if value := r.Header.Get("Range"); value != "" {
		var start, end int
		if _, err := fmt.Sscanf(value, "bytes=%d-%d", &start, &end); err != nil || start < 0 || end >= len(body) || end < start {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		body, status = body[start:end+1], http.StatusPartialContent
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *encryptionEndpoint) put(w http.ResponseWriter, r *http.Request) {
	s.puts++
	if s.retry {
		s.retry = false
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Error(err)
	}
	checksum := sha256.Sum256(body)
	if r.Header.Get("If-None-Match") != "*" || r.Header.Get("X-Amz-Checksum-Sha256") != base64.StdEncoding.EncodeToString(checksum[:]) {
		s.t.Error("encryption changed conditional write or checksum")
	}
	s.objects[r.URL.Path] = body
	w.WriteHeader(http.StatusOK)
}

func TestEncryptionPolicyFailureDoesNotDowngrade(t *testing.T) {
	t.Parallel()
	fixture := &encryptionEndpoint{t: t, mode: "aws:kms", key: "required-key", objects: make(map[string][]byte)}
	server := httptest.NewServer(fixture)
	defer server.Close()
	for _, mode := range []string{"", "AES256", "aws:kms"} {
		st, err := New(t.Context(), config.S3{Endpoint: server.URL, Bucket: "bucket", Region: "us-east-1",
			AccessKey: "test-key", SecretKey: "test-secret", UsePathStyle: true, SSE: mode})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Put(t.Context(), "pack", strings.NewReader("content"), 7); err == nil {
			t.Fatal("accepted missing or incorrect encryption policy headers")
		}
	}
	if _, err := New(t.Context(), config.S3{Bucket: "bucket", SSE: "invalid"}); err == nil {
		t.Fatal("direct constructor accepted invalid encryption")
	}
}
