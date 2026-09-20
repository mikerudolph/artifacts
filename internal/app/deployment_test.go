package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/testkit"
)

func TestBootstrap(t *testing.T) {
	t.Setenv("DATABASE_URL", testkit.Postgres(t))
	t.Setenv("ARTIFACTS_STORAGE", "invalid")
	t.Setenv("ARTIFACTS_BOOTSTRAP_TOKEN_FILE", "")
	secret := strings.Repeat("a", 48)
	t.Setenv("ARTIFACTS_BOOTSTRAP_TOKEN", secret)
	var out, diagnostic bytes.Buffer
	run := func(args ...string) int { return Run(context.Background(), args, &out, &diagnostic) }
	if run("bootstrap", "--account", "acme") != 1 {
		t.Fatal("accepted missing schema")
	}
	for range 2 {
		if run("migrate") != 0 {
			t.Fatal(diagnostic.String())
		}
	}
	for range 2 {
		if run("bootstrap", "--account", "acme") != 0 {
			t.Fatal(diagnostic.String())
		}
	}
	if run("bootstrap", "--account", "other") != 1 {
		t.Fatal("accepted conflicting account")
	}
	for _, args := range [][]string{{"bootstrap"}, {"bootstrap", "--bad"}, {"bootstrap", "--account", "acme", "extra"}} {
		if run(args...) != 2 {
			t.Fatal("accepted invalid arguments")
		}
	}
	if strings.Contains(out.String()+diagnostic.String(), secret) {
		t.Fatal("secret disclosed")
	}
}

func TestBootstrapSecret(t *testing.T) {
	t.Setenv("ARTIFACTS_BOOTSTRAP_TOKEN", "")
	t.Setenv("ARTIFACTS_BOOTSTRAP_TOKEN_FILE", "")
	if _, err := bootstrapSecret(); err == nil {
		t.Fatal("missing secret accepted")
	}
	path := filepath.Join(t.TempDir(), "token")
	t.Setenv("ARTIFACTS_BOOTSTRAP_TOKEN_FILE", path)
	if _, err := bootstrapSecret(); err == nil {
		t.Fatal("missing file accepted")
	}
	for _, value := range []string{strings.Repeat("x", 48) + "\n", "short", strings.Repeat("x", 4097), strings.Repeat("x", 32) + " space"} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := bootstrapSecret()
		valid := value == strings.Repeat("x", 48)+"\n"
		if (err == nil) != valid || valid && got != strings.TrimSpace(value) {
			t.Fatal("incorrect file validation")
		}
	}
	t.Setenv("ARTIFACTS_BOOTSTRAP_TOKEN", strings.Repeat("a", 32))
	if _, err := bootstrapSecret(); err == nil {
		t.Fatal("ambiguous source accepted")
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()
	h := &maintainedHandler{Handler: http.NotFoundHandler(), ready: func(context.Context) error { return errors.New("private detail") }}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/healthz", 200}, {"GET", "/readyz", 503}, {"HEAD", "/healthz", 200}, {"POST", "/readyz", 405}, {"GET", "/unknown", 404}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Code)
		}
	}
	h.ready = func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("unbounded readiness")
		}
		return nil
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	h.draining.Store(true)
	for _, path := range []string{"/readyz", "/anything"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
	if err := writableDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := writableDirectory(filepath.Join("/dev/null", "impossible")); err == nil {
		t.Fatal("accepted unwritable directory")
	}
}

func TestShutdown(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "drain", true: "deadline"}[deadline], func(t *testing.T) {
			t.Parallel()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
			h := &maintainedHandler{shutdownTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				select {
				case <-release:
					w.WriteHeader(204)
				case <-r.Context().Done():
					close(canceled)
				}
			})}
			if deadline {
				h.shutdownTimeout = 30 * time.Millisecond
			}
			result := make(chan int, 1)
			go func() { result <- serveListener(ctx, listener, h, io.Discard) }()
			response := make(chan int, 1)
			go func() {
				r, e := http.Get("http://" + listener.Addr().String())
				if e != nil {
					response <- 0
					return
				}
				_ = r.Body.Close()
				response <- r.StatusCode
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not arrive")
			}
			cancel()
			if !deadline {
				time.Sleep(20 * time.Millisecond)
				close(release)
			}
			assertShutdown(t, deadline, result, canceled, response)
		})
	}
}

func assertShutdown(t *testing.T, deadline bool, result <-chan int, canceled <-chan struct{}, response <-chan int) {
	t.Helper()
	select {
	case code := <-result:
		if code != map[bool]int{false: 0, true: 1}[deadline] {
			t.Fatal(code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung")
	}
	if deadline {
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("request not canceled")
		}
	} else if <-response != 204 {
		t.Fatal("request did not drain")
	}
}
