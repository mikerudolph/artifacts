package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) handleLog(w http.ResponseWriter, r *http.Request) {
	var out []types.LogEntry
	err := s.readRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"),
		func(st storer.Storer) error {
			var err error
			out, err = readLog(st, r.URL.Query().Get("ref"), queryInt(r.URL.Query().Get("limit"), 20), queryInt(r.URL.Query().Get("offset"), 0))
			return err
		})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}

func readLog(st storer.Storer, ref string, limit, offset int) ([]types.LogEntry, error) {
	h, err := resolveRef(st, ref)
	if err != nil {
		return nil, err
	}
	c, err := object.GetCommit(st, h)
	if err != nil {
		return nil, err
	}
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
		return nil, err
	}
	return out, nil
}
