package app

import (
	"context"
	"net/http"
	"os"
	"time"
)

func (h *maintainedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
		h.health(w, r)
		return
	}
	if h.draining.Load() {
		http.Error(w, "server is draining", http.StatusServiceUnavailable)
		return
	}
	h.Handler.ServeHTTP(w, r)
}

func (h *maintainedHandler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/readyz" && !h.isReady(r.Context()) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func writableDirectory(path string) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}
	file, err := os.CreateTemp(path, ".artifacts-write-check-*")
	if err != nil {
		return err
	}
	name := file.Name()
	err = file.Close()
	removeErr := os.Remove(name)
	if err != nil {
		return err
	}
	return removeErr
}

func (h *maintainedHandler) isReady(ctx context.Context) bool {
	if h.draining.Load() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return h.ready == nil || h.ready(ctx) == nil
}
