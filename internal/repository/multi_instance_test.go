package repository

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

type advancingObjects struct {
	*objecttest.Mem
	advance func()
}

func (s *advancingObjects) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.advance != nil {
		advance := s.advance
		s.advance = nil
		advance()
	}
	return s.Mem.Get(ctx, key)
}

func TestIndependentCacheSnapshotDuringPublication(t *testing.T) {
	ctx := context.Background()
	metadata, ns := forkFixture(t, ctx)
	for _, remote := range []bool{false, true} {
		name := "disk"
		if remote {
			name = "remote"
		}
		t.Run(name, func(t *testing.T) {
			objects := objecttest.NewMem()
			repo, err := metadata.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: types.RepoName(name)})
			if err != nil {
				t.Fatal(err)
			}
			writer, err := New(metadata, objects, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			first, err := writer.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "README.md", Content: "hello\n"}}})
			if err != nil {
				t.Fatal(err)
			}
			hooked := &advancingObjects{Mem: objects}
			reader, err := New(metadata, hooked, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			hooked.advance = func() {
				_, err := writer.Commit(ctx, repo, types.CommitInput{Files: []types.CommitFile{{Path: "new", Content: "later"}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := writer.Compact(ctx, repo); err != nil {
					t.Fatal(err)
				}
			}
			read := reader.Read
			if remote {
				read = reader.ReadContent
			}
			assertSnapshotHead(t, ctx, read, repo, first.SHA, true)
			if hooked.advance != nil {
				t.Fatal("did not overlap publication with reconstruction")
			}
			assertSnapshotHead(t, ctx, read, repo, first.SHA, false)
		})
	}
}

type publicationBarrier struct {
	meta.V2Store
	arrived chan struct{}
	release chan struct{}
}

func (s *publicationBarrier) RepositorySnapshot(ctx context.Context, id types.RepoID) (types.Repo, []types.Ref, error) {
	return s.V2Store.(meta.SnapshotReader).RepositorySnapshot(ctx, id)
}

func (s *publicationBarrier) WAL() meta.WAL {
	return barrierWAL{WAL: s.V2Store.WAL(), barrier: s}
}

type barrierWAL struct {
	meta.WAL
	barrier *publicationBarrier
}

func (s barrierWAL) Publish(ctx context.Context, p types.Publication) (int64, error) {
	s.barrier.arrived <- struct{}{}
	select {
	case <-s.barrier.release:
		return s.WAL.Publish(ctx, p)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func TestIndependentWritersConflictAndRecover(t *testing.T) {
	ctx := t.Context()
	metadata, ns := forkFixture(t, ctx)
	objects := objecttest.NewMem()
	repo, err := metadata.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "competing"})
	if err != nil {
		t.Fatal(err)
	}
	barrier := &publicationBarrier{V2Store: metadata, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	managers := make([]*Manager, 2)
	for i := range managers {
		managers[i], err = New(barrier, objects, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
	}
	results := make([]types.CommitResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range managers {
		wg.Go(func() {
			results[i], errs[i] = managers[i].Commit(ctx, repo, types.CommitInput{
				Files: []types.CommitFile{{Path: "file", Content: string(rune('a' + i))}},
			})
		})
	}
	for range managers {
		<-barrier.arrived
	}
	close(barrier.release)
	wg.Wait()
	winner, loser := 0, 1
	if errs[0] != nil {
		winner, loser = 1, 0
	}
	if errs[winner] != nil || !meta.IsCASConflict(errs[loser]) {
		t.Fatalf("publication errors: %v", errs)
	}
	managers[loser].meta = metadata
	next, err := managers[loser].Commit(ctx, repo, types.CommitInput{
		ExpectedHead: &results[winner].SHA, Files: []types.CommitFile{{Path: "retry", Content: "safe"}},
	})
	if err != nil || next.Sequence != 2 {
		t.Fatalf("retry: %+v %v", next, err)
	}
	for _, m := range managers {
		m.meta = metadata
		if err := m.Evict(repo); err != nil {
			t.Fatal(err)
		}
		assertSnapshotHead(t, ctx, m.Read, repo, next.SHA, true)
	}
	events, err := metadata.(meta.PublicationReader).PublicationEvents(ctx, repo.ID, 0, 100)
	if err != nil || len(events) != 2 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
}

func TestIndependentCacheIdempotency(t *testing.T) {
	ctx := t.Context()
	metadata, ns := forkFixture(t, ctx)
	repo, err := metadata.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "idempotent"})
	if err != nil {
		t.Fatal(err)
	}
	objects := objecttest.NewMem()
	managers := make([]*Manager, 2)
	for i := range managers {
		managers[i], err = New(metadata, objects, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
	}
	input := types.CommitInput{IdempotencyKey: "same-operation", Files: []types.CommitFile{{Path: "README.md", Content: "hello\n"}}}
	var wg sync.WaitGroup
	results := make([]types.CommitResult, 2)
	errs := make([]error, 2)
	for i := range managers {
		wg.Go(func() { results[i], errs[i] = managers[i].Commit(ctx, repo, input) })
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || results[0] != results[1] || results[0].Sequence != 1 {
		t.Fatalf("results=%+v errors=%v", results, errs)
	}
	assertReadme(t, managers[1], repo)
	input.Files[0].Content = "different"
	if _, err := managers[0].Commit(ctx, repo, input); !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("changed retry: %v", err)
	}
}

func assertSnapshotHead(t *testing.T, ctx context.Context, read func(context.Context, types.Repo, func(storer.Storer) error) error, repo types.Repo, sha string, equal bool) {
	t.Helper()
	err := read(ctx, repo, func(store storer.Storer) error {
		ref, err := store.Reference("refs/heads/main")
		if err != nil {
			return err
		}
		if (ref.Hash().String() == sha) != equal {
			return errors.New("unexpected snapshot head")
		}
		return store.HasEncodedObject(ref.Hash())
	})
	if err != nil {
		t.Fatal(err)
	}
}
