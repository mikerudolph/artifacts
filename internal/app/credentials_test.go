package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type pruningFunc func(context.Context, time.Time) (int64, error)

func (f pruningFunc) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	return f(ctx, cutoff)
}

func TestCredentialMaintenanceRetriesAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls atomic.Int32
	done := make(chan struct{})
	pruner := pruningFunc(func(operation context.Context, cutoff time.Time) (int64, error) {
		if age := time.Since(cutoff); age < time.Hour || age > time.Hour+time.Second {
			t.Error("wrong retention cutoff")
		}
		deadline, ok := operation.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("missing operation deadline")
		}
		switch calls.Add(1) {
		case 1:
			return 0, errors.New("transient failure")
		case 2:
			return 1000, nil
		default:
			cancel()
			return 0, operation.Err()
		}
	})
	go func() {
		defer close(done)
		maintainCredentials(ctx, pruner, time.Hour, time.Millisecond)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup loop did not stop")
	}
	if calls.Load() != 3 {
		t.Fatal("cleanup failed to retry on later ticks")
	}
}

func TestCredentialMaintenanceStopsBeforeClosing(t *testing.T) {
	started, exited := make(chan struct{}), make(chan struct{})
	h := &maintainedHandler{credentialRetention: time.Hour, credentials: pruningFunc(func(ctx context.Context, _ time.Time) (int64, error) {
		close(started)
		<-ctx.Done()
		close(exited)
		return 0, ctx.Err()
	})}
	stop := startMaintenance(t.Context(), h)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	stop()
	stop()
	select {
	case <-exited:
	default:
		t.Fatal("maintenance stopped before worker exited")
	}
}

func TestCredentialMaintenanceDisabled(t *testing.T) {
	h := &maintainedHandler{credentials: pruningFunc(func(context.Context, time.Time) (int64, error) {
		t.Error("disabled cleanup ran")
		return 0, nil
	})}
	startMaintenance(t.Context(), h)()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	maintainCredentials(ctx, h.credentials, time.Hour, time.Millisecond)
}
