package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestRepositorySnapshotTracksAtomicPublications(t *testing.T) {
	ctx, st, ns := v2Fixture(t)
	repo, err := st.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	reader := st.(meta.SnapshotReader)
	if _, _, err := reader.RepositorySnapshot(ctx, "missing"); !meta.IsNotFound(err) {
		t.Fatal(err)
	}
	current, refs, err := reader.RepositorySnapshot(ctx, repo.ID)
	if err != nil || len(refs) != 0 || current.AccountID != repo.AccountID {
		t.Fatalf("empty snapshot: %+v %v", current, err)
	}
	done := make(chan error, 1)
	go func() { done <- publishSnapshotSequence(ctx, st, repo.ID) }()
	for range 60 {
		current, refs, err := reader.RepositorySnapshot(ctx, repo.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertSnapshotSequence(t, current, refs)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := st.RunInTx(ctx, func(tx meta.Store) error {
		current, _, err := tx.(meta.SnapshotReader).RepositorySnapshot(ctx, repo.ID)
		if err == nil && current.WALSequence != 30 {
			return errors.New("transaction snapshot lost publications")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCompactionCoordinationReleasesOnFailure(t *testing.T) {
	ctx, st, ns := v2Fixture(t)
	repo, err := st.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	coordinator := st.(meta.CompactionCoordinator)
	boom := errors.New("upload failed")
	err = coordinator.RunCompaction(ctx, repo.ID, func(tx meta.V2Store) error {
		if err := coordinator.RunCompaction(ctx, repo.ID, func(meta.V2Store) error {
			t.Error("overlapping compaction acquired lock")
			return nil
		}); err != nil {
			return err
		}
		if _, err := st.WAL().Publish(ctx, types.Publication{Pack: types.PackWAL{RepoID: repo.ID, PackKey: "pack", IndexKey: "idx"}}); err != nil {
			return err
		}
		if _, _, err := tx.(meta.SnapshotReader).RepositorySnapshot(ctx, repo.ID); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	called := false
	err = coordinator.RunCompaction(ctx, repo.ID, func(meta.V2Store) error { called = true; return nil })
	if err != nil || !called {
		t.Fatalf("lock not released: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := coordinator.RunCompaction(canceled, repo.ID, func(meta.V2Store) error { return nil }); err == nil {
		t.Fatal("canceled compaction succeeded")
	}
}

func publishSnapshotSequence(ctx context.Context, st meta.V2Store, id types.RepoID) error {
	for sequence := int64(0); sequence < 30; sequence++ {
		old := ""
		if sequence > 0 {
			old = fmt.Sprint(sequence)
		}
		_, err := st.WAL().Publish(ctx, types.Publication{
			Pack:             types.PackWAL{RepoID: id, PackKey: "pack", IndexKey: "index"},
			ExpectedSequence: sequence,
			Updates:          []types.RefUpdate{{Name: "refs/heads/main", OldSHA: old, NewSHA: fmt.Sprint(sequence + 1)}},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func assertSnapshotSequence(t *testing.T, repo types.Repo, refs []types.Ref) {
	t.Helper()
	if repo.WALSequence == 0 {
		if len(refs) != 0 {
			t.Fatal("unpublished refs visible")
		}
	} else if len(refs) != 1 || refs[0].SHA != fmt.Sprint(repo.WALSequence) || refs[0].RepoID != repo.ID {
		t.Fatalf("inconsistent snapshot: sequence=%d refs=%+v", repo.WALSequence, refs)
	}
}
