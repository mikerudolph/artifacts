package repository

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (m *Manager) Events(ctx context.Context, repo types.Repo, after int64, limit int) (types.EventPage, error) {
	page := types.EventPage{Events: []types.PublicationEvent{}, NextAfter: after}
	if after < 0 || limit < 1 || limit > 100 {
		return page, &types.InputError{Field: "/after", Message: "after must be nonnegative and limit must be between 1 and 100"}
	}
	reader, ok := m.meta.(meta.PublicationReader)
	if !ok {
		return page, errors.New("publication event reader unavailable")
	}
	events, err := reader.PublicationEvents(ctx, repo.ID, after, limit)
	if err != nil {
		return page, err
	}
	page.Events = events
	if len(events) > 0 {
		page.NextAfter = events[len(events)-1].Sequence
	}
	return page, nil
}

func (m *Manager) Maintain(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := m.CompactPending(ctx, 32); err != nil && ctx.Err() == nil {
			slog.Error("repository compaction", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) CompactPending(ctx context.Context, threshold int64) error {
	source, ok := m.meta.(meta.CompactionSource)
	if !ok {
		return errors.New("compaction source unavailable")
	}
	ids, err := source.CompactionCandidates(ctx, threshold, 8)
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		repo, err := m.meta.Repos().GetByID(ctx, id)
		if err == nil {
			err = m.Compact(ctx, repo)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (m *Manager) currentRepo(ctx context.Context, repo types.Repo) (types.Repo, error) {
	if m.meta.Repos() == nil {
		return repo, nil
	}
	return m.meta.Repos().GetByID(ctx, repo.ID)
}
