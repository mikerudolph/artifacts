package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) handleTree(w http.ResponseWriter, r *http.Request) {
	var out []types.TreeEntry
	err := s.readRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"),
		func(st storer.Storer) error {
			tr, err := object.GetTree(st, plumbing.NewHash(chi.URLParam(r, "hash")))
			if err != nil {
				return meta.ErrNotFound
			}
			out = make([]types.TreeEntry, 0, len(tr.Entries))
			for _, e := range tr.Entries {
				kind := "blob"
				if e.Mode == filemode.Dir {
					kind = "tree"
				}
				out = append(out, types.TreeEntry{Mode: e.Mode.String(), Type: kind, Hash: e.Hash.String(), Name: e.Name})
			}
			return nil
		})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}
