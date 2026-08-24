package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func handleFile(w http.ResponseWriter, r *http.Request) {
	st, err := openRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	blob, err := fileAt(st, r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, err)
		return
	}
	rc, err := blob.Reader()
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
