package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/api/envelope"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth.Mode == "none" {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		authorized := subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Auth.APIToken)) == 1
		if s.deps.Authorizer != nil {
			account := types.AccountID(chi.URLParam(r, "account_id"))
			authorized = got != "" && s.deps.Authorizer.Authorize(r.Context(), account, got) == nil
		}
		if !authorized {
			_, body := envelope.Fail(http.StatusUnauthorized, envelope.APIError{
				Code:    envelope.CodeInvalidInput,
				Message: "unauthorized",
			})
			envelope.Write(w, http.StatusUnauthorized, body)
			return
		}
		next.ServeHTTP(w, r)
	})
}
