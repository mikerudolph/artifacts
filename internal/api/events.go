package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.deps.Repository.(interface {
		Events(context.Context, types.Repo, int64, int) (types.EventPage, error)
	})
	if !ok {
		writeErr(w, errJobs)
		return
	}
	after, err := eventParameter(r, "after", 0)
	if err != nil {
		writeErr(w, err)
		return
	}
	limit, err := eventParameter(r, "limit", 100)
	if err != nil {
		writeErr(w, err)
		return
	}
	if limit < 1 || limit > 100 {
		writeErr(w, &types.InputError{Field: "/limit", Message: "limit must be between 1 and 100"})
		return
	}
	repo, err := s.routeRepo(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	page, err := reader.Events(r.Context(), repo, after, int(limit))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, page)
}

func eventParameter(r *http.Request, name string, fallback int64) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, &types.InputError{Field: "/" + name, Message: name + " must be a nonnegative integer"}
	}
	return value, nil
}
