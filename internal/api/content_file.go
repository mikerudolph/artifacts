package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/httpstream"
)

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	stream := httpstream.New(w, s.deps.StreamIdle)
	defer stream.Close()
	output := stream.Writer(w)
	committed := false
	err := s.readRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"),
		func(st storer.Storer) error {
			blob, err := fileAt(st, r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
			if err != nil {
				return err
			}
			rc, err := blob.Reader()
			if err != nil {
				return err
			}
			defer func() { _ = rc.Close() }()
			output.Header().Set("Content-Type", "application/octet-stream")
			output.WriteHeader(http.StatusOK)
			committed = true
			_, err = io.Copy(output, rc)
			return err
		})
	if err != nil && !committed {
		writeErr(w, err)
	}
}
