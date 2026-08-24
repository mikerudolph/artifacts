package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/types"
)

type createNSBody struct {
	Namespace    string `json:"namespace"`
	Jurisdiction string `json:"jurisdiction"`
}

func (s *server) createNamespace(w http.ResponseWriter, r *http.Request) {
	var body createNSBody
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	j, err := types.ParseJurisdiction(body.Jurisdiction)
	if err != nil {
		writeErr(w, err)
		return
	}
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	ns, err := s.svc.CreateNamespace(r.Context(), acct, body.Namespace, j)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, ns)
}

func (s *server) getNamespace(w http.ResponseWriter, r *http.Request) {
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	ns, err := s.svc.GetNamespace(r.Context(), acct, chi.URLParam(r, "namespace"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, ns)
}

func (s *server) listNamespaces(w http.ResponseWriter, r *http.Request) {
	acct := types.AccountID(chi.URLParam(r, "account_id"))
	page := types.CursorPage{Cursor: r.URL.Query().Get("cursor")}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, _ := strconv.Atoi(v)
		page.Limit = n
	}
	list, info, err := s.svc.ListNamespaces(r.Context(), acct, page)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOKInfo(w, list, info)
}
