package repository

import (
	"context"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestDurableCompactionAndEvents(t *testing.T) {
	ctx := context.Background()
	metadata, ns := forkFixture(t, ctx)
	objects := objecttest.NewMem()
	m, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := metadata.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"one", "two"} {
		if _, err := m.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: content}}}); err != nil {
			t.Fatal(err)
		}
	}

	m, err = New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CompactPending(ctx, 2); err != nil {
		t.Fatal(err)
	}
	cp, err := metadata.Checkpoints().Get(ctx, repo.ID)
	if err != nil || cp.Sequence != 2 {
		t.Fatalf("checkpoint %+v %v", cp, err)
	}
	ids, err := metadata.(meta.CompactionSource).CompactionCandidates(ctx, 2, 8)
	if err != nil || len(ids) != 0 {
		t.Fatalf("compaction not complete: %v %v", ids, err)
	}
	if packs, err := metadata.WAL().List(ctx, repo.ID, 0, -1); err != nil || len(packs) != 2 {
		t.Fatal("compaction removed publication history")
	}
	page, err := m.Events(ctx, repo, 0, 100)
	if err != nil || len(page.Events) != 2 {
		t.Fatalf("events %+v %v", page, err)
	}
	testEventBounds(t, m, repo)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	m.Maintain(canceled)
}

func testEventBounds(t *testing.T, m *Manager, repo types.Repo) {
	t.Helper()
	ctx := context.Background()
	for _, args := range [][2]int64{{-1, 1}, {0, 0}, {0, 101}} {
		if _, err := m.Events(ctx, repo, args[0], int(args[1])); err == nil {
			t.Fatal("bad event bounds accepted")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	m.Maintain(canceled)
}
