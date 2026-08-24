package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) createRepo(w http.ResponseWriter, r *http.Request) {
	var in types.CreateRepoInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	got, err := s.svc.CreateRepo(r.Context(), acct, chi.URLParam(r, "namespace"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}

func (s *server) getRepo(w http.ResponseWriter, r *http.Request) {
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	got, err := s.svc.GetRepo(r.Context(), acct, chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}

func (s *server) listRepos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := types.CursorPage{Cursor: q.Get("cursor")}
	if v := q.Get("limit"); v != "" {
		n, _ := strconv.Atoi(v)
		page.Limit = n
	}
	sort, err := types.ParseRepoSortField(q.Get("sort"))
	if err != nil {
		writeErr(w, err)
		return
	}
	dir, err := types.ParseSortDirection(q.Get("direction"))
	if err != nil {
		writeErr(w, err)
		return
	}
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	list, info, err := s.svc.ListRepos(r.Context(), acct, chi.URLParam(r, "namespace"), meta.ListReposOpts{
		Search: q.Get("search"), Sort: sort, Direction: dir, Page: page,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOKInfo(w, list, info)
}

func (s *server) deleteRepo(w http.ResponseWriter, r *http.Request) {
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	id, err := s.svc.DeleteRepo(r.Context(), acct, chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusAccepted, map[string]string{"id": string(id)})
}
