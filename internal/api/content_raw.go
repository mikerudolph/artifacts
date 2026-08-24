package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func handleRaw(w http.ResponseWriter, r *http.Request) {
	st, err := openRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	path := chi.URLParam(r, "*")
	blob, err := fileAt(st, chi.URLParam(r, "ref"), path)
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
	buf := make([]byte, 512)
	n, _ := io.ReadFull(rc, buf)
	ct := http.DetectContentType(buf[:n])
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf[:n])
	_, _ = io.Copy(w, rc)
}
