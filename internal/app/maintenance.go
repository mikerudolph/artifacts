package app

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/mikerudolph/artifacts/internal/repository"
)

type maintainedHandler struct {
	http.Handler
	manager         *repository.Manager
	ready           func(context.Context) error
	close           func()
	shutdownTimeout time.Duration
	draining        atomic.Bool
}

func startMaintenance(ctx context.Context, h http.Handler) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	if h, ok := h.(*maintainedHandler); ok && h.manager != nil {
		go h.manager.Maintain(ctx)
	}
	return cancel
}
