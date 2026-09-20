package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/api/envelope"
	"github.com/mikerudolph/artifacts/internal/types"
)

type repositoryCredentialKey struct{}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.controlAuthorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		unauthorized(w)
	})
}

func bearer(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (s *server) controlAuthorized(r *http.Request) bool {
	if s.cfg.Auth.Mode == "none" {
		return true
	}
	got := bearer(r)
	if got == "" {
		return false
	}
	if s.deps.Authorizer != nil {
		return s.deps.Authorizer.Authorize(r.Context(), types.AccountID(chi.URLParam(r, "account_id")), got) == nil
	}
	return s.cfg.Auth.APIToken != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Auth.APIToken)) == 1
}

func unauthorized(w http.ResponseWriter) {
	_, body := envelope.Fail(http.StatusUnauthorized, envelope.APIError{Code: envelope.CodeInvalidInput, Kind: "unauthorized", Message: "unauthorized"})
	envelope.Write(w, http.StatusUnauthorized, body)
}

func (s *server) contentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.controlAuthorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		if s.deps.RepoAuthorizer == nil || bearer(r) == "" {
			unauthorized(w)
			return
		}
		repo, err := s.routeRepo(r)
		if err != nil {
			unauthorized(w)
			return
		}
		write := r.Method != http.MethodGet && r.Method != http.MethodHead
		err = s.deps.RepoAuthorizer.Authorize(r.Context(), repo, bearer(r), write)
		if errors.Is(err, types.ErrForbidden) {
			writeErr(w, err)
			return
		}
		if err != nil {
			unauthorized(w)
			return
		}
		if write && repo.ReadOnly {
			writeErr(w, types.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), repositoryCredentialKey{}, true)))
	})
}
