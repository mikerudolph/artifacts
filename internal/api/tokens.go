package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) createToken(w http.ResponseWriter, r *http.Request) {
	var in types.CreateTokenInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	got, err := s.svc.CreateToken(r.Context(), acct, chi.URLParam(r, "namespace"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}

func (s *server) listTokens(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state, err := types.ParseTokenListState(q.Get("state"))
	if err != nil {
		writeErr(w, err)
		return
	}
	page := types.OffsetPage{}
	if v := q.Get("page"); v != "" {
		page.Page, _ = strconv.Atoi(v)
	}
	if v := q.Get("per_page"); v != "" {
		page.PerPage, _ = strconv.Atoi(v)
	}
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	list, info, err := s.svc.ListTokens(r.Context(), acct, chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), state, page)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOKInfo(w, list, info)
}

func (s *server) revokeToken(w http.ResponseWriter, r *http.Request) {
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	err := s.svc.RevokeToken(r.Context(), acct, chi.URLParam(r, "namespace"), types.TokenID(chi.URLParam(r, "id")))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]string{"id": chi.URLParam(r, "id")})
}
