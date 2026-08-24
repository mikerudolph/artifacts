package service

import (
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
)

// Services is the control-plane application layer.
type Services struct {
	meta      meta.Store
	now       func() time.Time
	publicURL string
}

// New constructs Services. now may be nil (defaults to time.Now).
func New(m meta.Store, now func() time.Time, publicURL string) *Services {
	if now == nil {
		now = time.Now
	}
	return &Services{meta: m, now: now, publicURL: publicURL}
}
