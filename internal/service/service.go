package service

import (
	"time"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/store/meta"
)

type Services struct {
	meta      meta.Store
	now       func() time.Time
	publicURL string
	issuer    auth.CredentialIssuer
}

func New(m meta.Store, now func() time.Time, publicURL string) *Services {
	if now == nil {
		now = time.Now
	}
	return &Services{meta: m, now: now, publicURL: publicURL, issuer: auth.NewCredentialIssuer(m.RepoTokens(), now)}
}

func NewWithIssuer(m meta.Store, now func() time.Time, publicURL string, issuer auth.CredentialIssuer) *Services {
	s := New(m, now, publicURL)
	if issuer != nil {
		s.issuer = issuer
	}
	return s
}
