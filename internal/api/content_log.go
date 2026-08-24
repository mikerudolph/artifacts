package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/types"
)

func handleLog(w http.ResponseWriter, r *http.Request) {
	st, err := openRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	h, err := resolveRef(st, r.URL.Query().Get("ref"))
	if err != nil {
		writeErr(w, err)
		return
	}
	c, err := object.GetCommit(st, h)
	if err != nil {
		writeErr(w, err)
		return
	}
	limit := queryInt(r.URL.Query().Get("limit"), 20)
	offset := queryInt(r.URL.Query().Get("offset"), 0)
	var out []types.LogEntry
	n := 0
	err = object.NewCommitPreorderIter(c, nil, nil).ForEach(func(cm *object.Commit) error {
		if n < offset {
			n++
			return nil
		}
		if len(out) >= limit {
			return storer.ErrStop
		}
		out = append(out, types.LogEntry{
			Hash:    cm.Hash.String(),
			Message: cm.Message,
			Author:  types.Signature{Name: cm.Author.Name, Email: cm.Author.Email, When: cm.Author.When},
			Committer: types.Signature{
				Name: cm.Committer.Name, Email: cm.Committer.Email, When: cm.Committer.When,
			},
		})
		n++
		return nil
	})
	if err != nil && err != storer.ErrStop {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}
