package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func handleTree(w http.ResponseWriter, r *http.Request) {
	st, err := openRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	tr, err := object.GetTree(st, plumbing.NewHash(chi.URLParam(r, "hash")))
	if err != nil {
		writeErr(w, meta.ErrNotFound)
		return
	}
	out := make([]types.TreeEntry, 0, len(tr.Entries))
	for _, e := range tr.Entries {
		kind := "blob"
		if e.Mode == filemode.Dir {
			kind = "tree"
		}
		out = append(out, types.TreeEntry{
			Mode: e.Mode.String(),
			Type: kind,
			Hash: e.Hash.String(),
			Name: e.Name,
		})
	}
	writeOK(w, http.StatusOK, out)
}
