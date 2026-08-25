package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func v2Fixture(t *testing.T) (context.Context, meta.V2Store, types.Namespace) {
	t.Helper()
	ctx := context.Background()
	dsn := testkit.Postgres(t)
	if err := Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closer, ok := store.(interface{ Close() }); ok {
			closer.Close()
		}
	})
	if err := store.Accounts().Ensure(ctx, "account-a"); err != nil {
		t.Fatal(err)
	}
	ns, err := store.Namespaces().Create(ctx, types.Namespace{AccountID: "account-a", Name: "agents"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, ns
}

func TestAtomicPublicationRollbackAndState(t *testing.T) {
	ctx, store, ns := v2Fixture(t)
	repo, err := store.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "app", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	first := types.Publication{Pack: types.PackWAL{RepoID: repo.ID, PackKey: "a/p1.pack", IndexKey: "a/p1.idx", Checksum: "one", Size: 1},
		Updates: []types.RefUpdate{{Name: "refs/heads/main", NewSHA: "aaa"}, {Name: "refs/heads/other", NewSHA: "bbb"}}}
	sequence, err := store.WAL().Publish(ctx, first)
	if err != nil || sequence != 1 {
		t.Fatalf("publish %d %v", sequence, err)
	}
	bad := types.Publication{ExpectedSequence: 1,
		Pack: types.PackWAL{RepoID: repo.ID, PackKey: "a/p2.pack", IndexKey: "a/p2.idx", Checksum: "two", Size: 2},
		Updates: []types.RefUpdate{{Name: "refs/heads/main", OldSHA: "aaa", NewSHA: "ccc"},
			{Name: "refs/heads/other", OldSHA: "wrong", NewSHA: "ddd"}}}
	if _, err := store.WAL().Publish(ctx, bad); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("bad publication: %v", err)
	}
	main, _ := store.Refs().Get(ctx, repo.ID, "refs/heads/main")
	other, _ := store.Refs().Get(ctx, repo.ID, "refs/heads/other")
	got, _ := store.Repos().GetByID(ctx, repo.ID)
	if main.SHA != "aaa" || other.SHA != "bbb" || got.WALSequence != 1 {
		t.Fatalf("partial publication: %q %q %d", main.SHA, other.SHA, got.WALSequence)
	}
	got.ReadOnly = true
	if _, err := store.Repos().Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	bad.ExpectedSequence = 1
	bad.Updates = []types.RefUpdate{{Name: main.Name, OldSHA: main.SHA, NewSHA: "ccc"}}
	if _, err := store.WAL().Publish(ctx, bad); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("read-only publication: %v", err)
	}
}

func TestSnapshotForkCapturesSequenceAndRefs(t *testing.T) {
	ctx, store, ns := v2Fixture(t)
	source, err := store.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "source", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.WAL().Publish(ctx, types.Publication{
		Pack:    types.PackWAL{RepoID: source.ID, PackKey: "a/source1.pack", IndexKey: "a/source1.idx", Checksum: "one"},
		Updates: []types.RefUpdate{{Name: "refs/heads/main", NewSHA: "aaa"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	dest, err := store.Forks().CreateSnapshot(ctx, source.ID, types.Repo{NamespaceID: ns.ID, Name: "fork"}, false)
	if err != nil {
		t.Fatal(err)
	}
	line, err := store.Forks().Get(ctx, dest.ID)
	if err != nil || line.ParentSequence != 1 || line.ParentRepoID != source.ID {
		t.Fatalf("lineage %+v %v", line, err)
	}
	cp := types.Checkpoint{RepoID: source.ID, Sequence: 1, PackKey: "a/cp.pack", IndexKey: "a/cp.idx", Checksum: "checkpoint"}
	if err := store.Checkpoints().Put(ctx, cp); err != nil {
		t.Fatal(err)
	}
	gotCP, err := store.Checkpoints().Get(ctx, source.ID)
	if err != nil || gotCP.Sequence != 1 {
		t.Fatalf("checkpoint %+v %v", gotCP, err)
	}
	_, err = store.WAL().Publish(ctx, types.Publication{ExpectedSequence: 1,
		Pack:    types.PackWAL{RepoID: source.ID, PackKey: "a/source2.pack", IndexKey: "a/source2.idx", Checksum: "two"},
		Updates: []types.RefUpdate{{Name: "refs/heads/main", OldSHA: "aaa", NewSHA: "bbb"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := store.Refs().Get(ctx, dest.ID, "refs/heads/main")
	children, _ := store.Forks().Children(ctx, source.ID)
	if ref.SHA != "aaa" || len(children) != 1 {
		t.Fatalf("fork moved: %q children=%d", ref.SHA, len(children))
	}
}

func TestZeroWatermarkAndRepeatedPackMetadata(t *testing.T) {
	ctx, store, ns := v2Fixture(t)
	repo, err := store.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "watermark", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	pack := types.PackWAL{RepoID: repo.ID, PackKey: "same/content.pack", IndexKey: "same/content.idx", Checksum: "same"}
	if _, err := store.WAL().Publish(ctx, types.Publication{Pack: pack,
		Updates: []types.RefUpdate{{Name: "refs/heads/main", NewSHA: "aaa"}}}); err != nil {
		t.Fatal(err)
	}
	if packs, err := store.WAL().List(ctx, repo.ID, 0, 0); err != nil || len(packs) != 0 {
		t.Fatalf("zero watermark packs=%+v err=%v", packs, err)
	}
	if packs, err := store.WAL().List(ctx, repo.ID, 0, -1); err != nil || len(packs) != 1 {
		t.Fatalf("unbounded packs=%+v err=%v", packs, err)
	}
	_, err = store.WAL().Publish(ctx, types.Publication{Pack: pack, ExpectedSequence: 1,
		Updates: []types.RefUpdate{{Name: "refs/heads/feature", NewSHA: "aaa"}}})
	if err != nil {
		t.Fatalf("reused immutable pack metadata: %v", err)
	}
	_, err = store.WAL().Publish(ctx, types.Publication{Pack: pack, ExpectedSequence: 2,
		Updates: []types.RefUpdate{{Name: "refs/heads/feature", OldSHA: "aaa"}}})
	if err != nil {
		t.Fatalf("ref-only deletion with reused pack: %v", err)
	}
}

func TestRepoCursorAllSorts(t *testing.T) {
	ctx, store, ns := v2Fixture(t)
	for i := 0; i < 7; i++ {
		_, err := store.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: types.RepoName(fmt.Sprintf("repo-%d", i))})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, sort := range []types.RepoSortField{types.SortCreatedAt, types.SortUpdatedAt, types.SortLastPushAt, types.SortName} {
		for _, direction := range []types.SortDirection{types.SortAsc, types.SortDesc} {
			seen := map[types.RepoID]bool{}
			cursor := ""
			for {
				list, info, err := store.Repos().List(ctx, meta.ListReposOpts{NamespaceID: ns.ID, Sort: sort, Direction: direction,
					Page: types.CursorPage{Limit: 2, Cursor: cursor}})
				if err != nil {
					t.Fatal(err)
				}
				for _, repo := range list {
					if seen[repo.ID] {
						t.Fatalf("duplicate %s for %s/%s", repo.ID, sort, direction)
					}
					seen[repo.ID] = true
				}
				cursor = info.Cursor
				if cursor == "" {
					break
				}
			}
			if len(seen) != 7 {
				t.Fatalf("%s/%s returned %d", sort, direction, len(seen))
			}
		}
	}
	if _, _, err := store.Repos().List(ctx, meta.ListReposOpts{NamespaceID: ns.ID, Page: types.CursorPage{Cursor: "bad"}}); err == nil {
		t.Fatal("accepted invalid repo cursor")
	}
}
