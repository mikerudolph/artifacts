package app

import (
	"context"
	"net/http"

	"github.com/mikerudolph/artifacts/internal/repository"
)

type maintainedHandler struct {
	http.Handler
	manager *repository.Manager
}

func startMaintenance(ctx context.Context, h http.Handler) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	if h, ok := h.(*maintainedHandler); ok {
		go h.manager.Maintain(ctx)
	}
	return cancel
}
