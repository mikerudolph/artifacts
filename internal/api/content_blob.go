package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/mikerudolph/artifacts/internal/store/meta"
)

func handleBlob(w http.ResponseWriter, r *http.Request) {
	st, err := openRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	blob, err := object.GetBlob(st, plumbing.NewHash(chi.URLParam(r, "hash")))
	if err != nil {
		writeErr(w, meta.ErrNotFound)
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
