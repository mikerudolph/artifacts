package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/mikerudolph/artifacts/internal/api/envelope"
)

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth.Mode == "none" {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Auth.APIToken)) != 1 {
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
