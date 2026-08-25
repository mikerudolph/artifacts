package githttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

type routeLookup struct {
	repo types.Repo
	err  error
}

func (l routeLookup) Lookup(context.Context, types.AccountID, string, string) (types.Repo, error) {
	return l.repo, l.err
}

type routeAuth struct{ err error }

func (a routeAuth) Authorize(context.Context, types.Repo, string, bool) error { return a.err }

type routeBackend struct{}

func (routeBackend) RPC(_ context.Context, _ types.Repo, service string, input io.Reader, output io.Writer, _ string) error {
	if input == nil {
		_, _ = io.WriteString(output, "advertised")
		return nil
	}
	_, _ = io.WriteString(output, "uploaded-"+service)
	return nil
}
func (routeBackend) Receive(context.Context, types.Repo, io.Reader, string) ([]byte, error) {
	return []byte("received"), nil
}

func TestRepositoryRoutesAndAuthorization(t *testing.T) {
	repo := types.Repo{ID: "repo_1", AccountID: "acct", Status: types.RepoReady}
	h := NewRepository(routeBackend{}, routeLookup{repo: repo}, routeAuth{}, false)
	request := httptest.NewRequest(http.MethodGet, "/git/acct/ns/app.git/info/refs?service=git-upload-pack", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("advertised")) {
		t.Fatalf("advertise %d %q", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/git/acct/ns/app.git/git-receive-pack", bytes.NewReader(nil))
	request.Header.Set("Authorization", "Bearer token")
	recorder = httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "received" {
		t.Fatalf("receive %d %q", recorder.Code, recorder.Body.String())
	}

	cases := []struct {
		name string
		h    http.Handler
		want int
	}{
		{"missing repo", NewRepository(routeBackend{}, routeLookup{err: errors.New("missing")}, routeAuth{}, false), http.StatusNotFound},
		{"bad token", NewRepository(routeBackend{}, routeLookup{repo: repo}, routeAuth{err: errors.New("bad")}, false), http.StatusUnauthorized},
		{"read only", NewRepository(routeBackend{}, routeLookup{repo: types.Repo{Status: types.RepoReady, ReadOnly: true}}, routeAuth{}, false), http.StatusForbidden},
		{"not ready", NewRepository(routeBackend{}, routeLookup{repo: types.Repo{Status: types.RepoFailed}}, routeAuth{}, false), http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/git/acct/ns/app.git/git-receive-pack", nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			tc.h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d", w.Code)
			}
		})
	}
}

func TestRepositoryDevBypassAndBadService(t *testing.T) {
	repo := types.Repo{ID: "repo_1", Status: types.RepoReady}
	h := NewRepository(routeBackend{}, routeLookup{repo: repo}, nil, true)
	for path, want := range map[string]int{
		"/git/a/n/r.git/info/refs?service=nope": http.StatusForbidden,
		"/git/a/n/r.git/git-upload-pack":        http.StatusOK,
	} {
		method := http.MethodPost
		if strings.Contains(path, "info/refs") {
			method = http.MethodGet
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != want {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

type accountLookup struct{}

func (accountLookup) Lookup(_ context.Context, account types.AccountID, _, _ string) (types.Repo, error) {
	return types.Repo{ID: types.RepoID(account), AccountID: account, Status: types.RepoReady}, nil
}

type streamBackend struct {
	started  chan types.AccountID
	released chan types.AccountID
	canceled chan types.AccountID
}

func (b streamBackend) RPC(_ context.Context, _ types.Repo, _ string, _ io.Reader, output io.Writer, _ string) error {
	_, err := io.WriteString(output, "ok")
	return err
}

func (b streamBackend) Receive(ctx context.Context, repo types.Repo, input io.Reader, _ string) ([]byte, error) {
	b.started <- repo.AccountID
	_, err := io.Copy(io.Discard, input)
	if err != nil && ctx.Err() != nil {
		b.canceled <- repo.AccountID
	}
	b.released <- repo.AccountID
	return []byte("ok"), err
}

func TestStalledSameRepoStreamsReleaseSlots(t *testing.T) {
	backend := streamBackend{
		started: make(chan types.AccountID, 40), released: make(chan types.AccountID, 40), canceled: make(chan types.AccountID, 40),
	}
	server := httptest.NewServer(newRepository(backend, accountLookup{}, nil, true, 150*time.Millisecond))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	first := stalledReceive(t, host, "one", true)
	if account := waitAccount(t, backend.started); account != "one" {
		t.Fatalf("started %s", account)
	}
	waiters := make([]net.Conn, 31)
	for i := range waiters {
		waiters[i] = stalledReceive(t, host, "one", false)
	}
	response, err := http.Post(server.URL+"/git/two/ns/repo.git/git-upload-pack", "application/x-git-upload-pack-request", strings.NewReader("done"))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("other tenant status %d", response.StatusCode)
	}
	for _, conn := range waiters {
		_ = conn.Close()
	}
	if account := waitAccount(t, backend.released); account != "one" {
		t.Fatalf("released %s", account)
	}
	if account := waitAccount(t, backend.canceled); account != "one" {
		t.Fatalf("canceled %s", account)
	}
	_ = first.Close()
	response, err = http.Post(server.URL+"/git/one/ns/repo.git/git-upload-pack", "application/x-git-upload-pack-request", strings.NewReader("done"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("recovered status %d", response.StatusCode)
	}
}

type outputBackend struct {
	started, released, canceled chan types.AccountID
}

func (b outputBackend) RPC(ctx context.Context, repo types.Repo, _ string, _ io.Reader, output io.Writer, protocol string) error {
	if protocol == "quick" {
		_, err := io.WriteString(output, "ok")
		return err
	}
	b.started <- repo.AccountID
	chunk := make([]byte, 32<<10)
	for {
		if _, err := output.Write(chunk); err != nil {
			if ctx.Err() != nil {
				b.canceled <- repo.AccountID
			}
			b.released <- repo.AccountID
			return err
		}
	}
}

func (outputBackend) Receive(context.Context, types.Repo, io.Reader, string) ([]byte, error) {
	return nil, errors.New("not used")
}

func TestStalledUploadCancelsBackendAndReleasesRepo(t *testing.T) {
	backend := outputBackend{
		started: make(chan types.AccountID, 1), released: make(chan types.AccountID, 1), canceled: make(chan types.AccountID, 1),
	}
	server := httptest.NewServer(newRepository(backend, accountLookup{}, nil, true, 50*time.Millisecond))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(1024)
	}
	request := "POST /git/one/ns/repo.git/git-upload-pack HTTP/1.1\r\nHost: " + host + "\r\nContent-Length: 0\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	if account := waitAccount(t, backend.started); account != "one" {
		t.Fatalf("started %s", account)
	}
	if account := waitAccount(t, backend.released); account != "one" {
		t.Fatalf("released %s", account)
	}
	if account := waitAccount(t, backend.canceled); account != "one" {
		t.Fatalf("canceled %s", account)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/git/one/ns/repo.git/git-upload-pack", strings.NewReader("done"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Git-Protocol", "quick")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("recovered status %d", response.StatusCode)
	}
}

func stalledReceive(t *testing.T, host string, account types.AccountID, partial bool) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	if partial {
		body = "x"
	}
	request := "POST /git/" + string(account) + "/ns/repo.git/git-receive-pack HTTP/1.1\r\nHost: " + host + "\r\nContent-Length: 10\r\n\r\n" + body
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	return conn
}

func waitAccount(t *testing.T, ch <-chan types.AccountID) types.AccountID {
	t.Helper()
	select {
	case account := <-ch:
		return account
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream")
		return ""
	}
}
