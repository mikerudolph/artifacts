package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) handleCommit(w http.ResponseWriter, r *http.Request) {
	var result types.Commit
	err := s.readRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"),
		func(st storer.Storer) error {
			c, err := object.GetCommit(st, plumbing.NewHash(chi.URLParam(r, "hash")))
			if err != nil {
				return meta.ErrNotFound
			}
			parents := make([]string, 0, len(c.ParentHashes))
			for _, p := range c.ParentHashes {
				parents = append(parents, p.String())
			}
			result = types.Commit{Hash: c.Hash.String(), Tree: c.TreeHash.String(), Parents: parents,
				Author:    types.Signature{Name: c.Author.Name, Email: c.Author.Email, When: c.Author.When},
				Committer: types.Signature{Name: c.Committer.Name, Email: c.Committer.Email, When: c.Committer.When}, Message: c.Message}
			return nil
		})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, result)
}
