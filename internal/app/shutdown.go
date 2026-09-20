package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

func serveListener(ctx context.Context, listener net.Listener, h http.Handler, stderr io.Writer) int {
	defer func() { _ = listener.Close() }()
	defer closeHandler(h)
	stop := startMaintenance(ctx, h)
	defer stop()
	requests, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	srv := newHTTPServer("", h)
	srv.BaseContext = func(net.Listener) context.Context { return requests }
	finished := make(chan struct{})
	drained := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			stop()
			drained <- drainServer(srv, h)
		case <-finished:
			drained <- nil
		}
	}()
	_, _ = fmt.Fprintf(stderr, "listening on %s\n", listener.Addr())
	err := srv.Serve(listener)
	close(finished)
	drainErr := <-drained
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if drainErr != nil {
		_, _ = fmt.Fprintln(stderr, "shutdown deadline exceeded; active requests canceled")
		return 1
	}
	return 0
}

func drainServer(srv *http.Server, h http.Handler) error {
	timeout := 30 * time.Second
	if managed, ok := h.(*maintainedHandler); ok {
		managed.draining.Store(true)
		if managed.shutdownTimeout > 0 {
			timeout = managed.shutdownTimeout
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := srv.Shutdown(ctx)
	if err != nil {
		_ = srv.Close()
	}
	return err
}

func closeHandler(h http.Handler) {
	if managed, ok := h.(*maintainedHandler); ok && managed.close != nil {
		managed.close()
	}
}
