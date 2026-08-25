package app

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerReadTimeoutStopsSlowBody(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "body timeout", http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := newHTTPServer("", handler)
	server.ReadTimeout = 40 * time.Millisecond
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 10\r\n\r\nx")
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusRequestTimeout {
		t.Fatalf("slow body status %d", response.StatusCode)
	}
}

func TestServerAllowsSlowValidStreamingBody(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := newHTTPServer("", handler)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = io.WriteString(conn, "POST /git/a/n/r.git/git-upload-pack HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\n\r\na")
	time.Sleep(60 * time.Millisecond)
	_, _ = io.WriteString(conn, "bcd")
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("slow valid body status %d", response.StatusCode)
	}
}
