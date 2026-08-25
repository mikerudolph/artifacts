package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Jobs is the optional fork/import runner. Set by process wiring.
var Jobs *jobs.Runner

func (s *server) handleFork(w http.ResponseWriter, r *http.Request) {
	runner := s.deps.Jobs
	if runner == nil {
		runner = Jobs
	}
	if runner == nil {
		writeErr(w, errJobs)
		return
	}
	var in types.ForkRepoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	got, err := runner.Fork(r.Context(), types.AccountID(chi.URLParam(r, "account_id")), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}

func (s *server) handleImport(w http.ResponseWriter, r *http.Request) {
	runner := s.deps.Jobs
	if runner == nil {
		runner = Jobs
	}
	if runner == nil {
		writeErr(w, errJobs)
		return
	}
	var in types.ImportRepoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	name := types.RepoName(chi.URLParam(r, "name"))
	got, err := runner.Import(r.Context(), types.AccountID(chi.URLParam(r, "account_id")), chi.URLParam(r, "namespace"), name, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, got)
}
