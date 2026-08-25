package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) routeRepo(r *http.Request) (types.Repo, error) {
	return s.svc.GetRepo(r.Context(), types.AccountID(chi.URLParam(r, "account_id")),
		chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
}

func (s *server) createCommit(w http.ResponseWriter, r *http.Request) {
	if s.deps.Repository == nil {
		writeErr(w, errJobs)
		return
	}
	var input types.CommitInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	repo, err := s.routeRepo(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	result, err := s.deps.Repository.Commit(r.Context(), repo, input)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, result)
}

func (s *server) listRefs(w http.ResponseWriter, r *http.Request) {
	if s.deps.Repository == nil {
		writeErr(w, errJobs)
		return
	}
	repo, err := s.routeRepo(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	refs, err := s.deps.Repository.Refs(r.Context(), repo)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, refs)
}

func (s *server) listWAL(w http.ResponseWriter, r *http.Request) {
	if s.deps.Repository == nil {
		writeErr(w, errJobs)
		return
	}
	repo, err := s.routeRepo(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	packs, err := s.deps.Repository.WAL(r.Context(), repo)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, packs)
}

func (s *server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var input types.UpdateRepoInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeErr(w, types.ErrInvalidName)
		return
	}
	repo, err := s.svc.UpdateRepo(r.Context(), types.AccountID(chi.URLParam(r, "account_id")),
		chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), input)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, repo)
}

func (s *server) listJobs(w http.ResponseWriter, r *http.Request) {
	if s.deps.Jobs == nil {
		writeOK(w, http.StatusOK, []types.Job{})
		return
	}
	repo, err := s.routeRepo(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	jobs, err := s.deps.Jobs.List(r.Context(), repo.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, jobs)
}
