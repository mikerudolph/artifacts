package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mikerudolph/artifacts/internal/repository"
	"github.com/mikerudolph/artifacts/internal/store/meta"
)

type maintainedHandler struct {
	http.Handler
	manager             *repository.Manager
	ready               func(context.Context) error
	close               func()
	shutdownTimeout     time.Duration
	credentials         meta.CredentialPruner
	credentialRetention time.Duration
	draining            atomic.Bool
}

func startMaintenance(ctx context.Context, h http.Handler) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	if h, ok := h.(*maintainedHandler); ok {
		if h.manager != nil {
			workers.Go(func() { h.manager.Maintain(ctx) })
		}
		if h.credentials != nil && h.credentialRetention > 0 {
			workers.Go(func() { maintainCredentials(ctx, h.credentials, h.credentialRetention, time.Minute) })
		}
	}
	return func() {
		cancel()
		workers.Wait()
	}
}

func maintainCredentials(ctx context.Context, tokens meta.CredentialPruner, retention, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		pruneCredentials(ctx, tokens, time.Now().Add(-retention))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func pruneCredentials(ctx context.Context, tokens meta.CredentialPruner, cutoff time.Time) {
	operation, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	count, err := tokens.Prune(operation, cutoff)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		slog.Error("credential cleanup failed")
	} else if count > 0 {
		slog.Info("credential cleanup", "deleted", count)
	}
}
