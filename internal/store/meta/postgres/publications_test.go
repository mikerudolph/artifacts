package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestEventsOnlyExposeCommittedWholePublications(t *testing.T) {
	ctx, metadata, ns := v2Fixture(t)
	repo, err := metadata.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "events"})
	if err != nil {
		t.Fatal(err)
	}
	reader := metadata.(meta.PublicationReader)
	publication := types.Publication{Pack: types.PackWAL{RepoID: repo.ID, PackKey: "pack", IndexKey: "index"}, Updates: []types.RefUpdate{{Name: "refs/heads/main", NewSHA: "a"}, {Name: "refs/tags/v1", NewSHA: "b"}}}
	rollback := errors.New("abort transaction")
	err = metadata.RunInTx(ctx, func(tx meta.Store) error {
		if _, err := tx.(meta.V2Store).WAL().Publish(ctx, publication); err != nil {
			return err
		}
		events, err := reader.PublicationEvents(ctx, repo.ID, 0, 1)
		if err != nil {
			return err
		}
		if len(events) != 0 {
			t.Fatal("uncommitted event visible")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	events, err := reader.PublicationEvents(ctx, repo.ID, 0, 1)
	if err != nil || len(events) != 0 {
		t.Fatal("rolled-back event visible")
	}
	if _, err := metadata.WAL().Publish(ctx, publication); err != nil {
		t.Fatal(err)
	}
	events, err = reader.PublicationEvents(ctx, repo.ID, 0, 1)
	if err != nil || len(events) != 1 || len(events[0].Updates) != 2 {
		t.Fatalf("partial publication: %+v %v", events, err)
	}
	assertDeletionEvent(t, metadata, repo, publication)
}

func assertDeletionEvent(t *testing.T, metadata meta.V2Store, repo types.Repo, publication types.Publication) {
	t.Helper()
	ctx := context.Background()
	reader := metadata.(meta.PublicationReader)
	publication.ExpectedSequence = 1
	publication.Updates = []types.RefUpdate{{Name: "refs/tags/v1", OldSHA: "b", NewSHA: ""}}
	if _, err := metadata.WAL().Publish(ctx, publication); err != nil {
		t.Fatal(err)
	}
	events, err := reader.PublicationEvents(ctx, repo.ID, 1, 1)
	if err != nil || len(events) != 1 || events[0].Updates[0].NewSHA != "" {
		t.Fatalf("deletion event %+v %v", events, err)
	}
}
