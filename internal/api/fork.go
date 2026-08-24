package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Jobs is the optional fork/import runner. Set by process wiring.
var Jobs *jobs.Runner

func init() {
	jobRoutes = func(r chi.Router) {
		r.Post("/namespaces/{namespace}/repos/{name}/fork", handleFork)
		r.Post("/namespaces/{namespace}/repos/{name}/import", handleImport)
	}
}

func handleFork(w http.ResponseWriter, r *http.Request) {
	if Jobs == nil {
		writeErr(w, errJobs)
		return
	}
	var in types.ForkRepoInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	got, err := Jobs.Fork(r.Context(), types.AccountID(chi.URLParam(r, "account_id")), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	if Jobs == nil {
		writeErr(w, errJobs)
		return
	}
	var in types.ImportRepoInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	name := types.RepoName(chi.URLParam(r, "name"))
	got, err := Jobs.Import(r.Context(), types.AccountID(chi.URLParam(r, "account_id")), chi.URLParam(r, "namespace"), name, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}
