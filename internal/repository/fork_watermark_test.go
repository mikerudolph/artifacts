package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestEmptyForkDoesNotFollowFutureParentWAL(t *testing.T) {
	ctx := context.Background()
	metadata, ns := forkFixture(t, ctx)
	objects := objecttest.NewMem()
	manager, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := metadata.Repos().Create(ctx, types.Repo{
		NamespaceID: ns.ID, Name: "empty-source", DefaultBranch: "main", Status: types.RepoReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := jobs.NewWithPublisher(metadata, objects, "http://example.test", manager)
	forked, err := runner.Fork(ctx, "acct", "default", "empty-source", types.ForkRepoInput{Name: "empty-fork"})
	if err != nil {
		t.Fatal(err)
	}
	commit, err := manager.Commit(ctx, source, types.CommitInput{Files: []types.CommitFile{{Path: "future", Content: "hidden"}}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := metadata.Repos().GetByID(ctx, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.Read(ctx, child, func(store storer.Storer) error {
		if err := store.HasEncodedObject(plumbing.NewHash(commit.SHA)); err == nil {
			t.Fatal("empty fork inherited a future parent object")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLegacyForkUpgradesBeforeSnapshot(t *testing.T) {
	testkit.GitAvailable(t)
	ctx := context.Background()
	dsn := testkit.Postgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	execMigration(t, db, "001_init.up.sql")
	_, err = db.ExecContext(ctx, `INSERT INTO accounts(id) VALUES('acct');
		INSERT INTO namespaces(id,account_id,name) VALUES('ns','acct','default');
		INSERT INTO repos(id,namespace_id,name,default_branch,status) VALUES('legacy','ns','legacy','main','ready')`)
	if err != nil {
		t.Fatal(err)
	}
	execMigration(t, db, "002_v2_storage.up.sql")
	metadata, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	objects := objecttest.NewMem()
	writeLegacyRepository(t, ctx, metadata, objects)
	root := t.TempDir()
	manager, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	managers := []*Manager{manager, second}
	runners := []*jobs.Runner{
		jobs.NewWithPublisher(metadata, objects, "http://example.test", managers[0]),
		jobs.NewWithPublisher(metadata, objects, "http://example.test", managers[1]),
	}
	forks := make([]types.CreateRepoResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range runners {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			forks[index], errs[index] = runners[index].Fork(ctx, "acct", "default", "legacy", types.ForkRepoInput{Name: types.RepoName(fmt.Sprintf("legacy-fork-%d", index))})
		}(i)
	}
	wg.Wait()
	source, _ := metadata.Repos().GetByID(ctx, "legacy")
	if source.StorageVersion != 2 || source.WALSequence != 1 {
		t.Fatalf("source not upgraded: %+v", source)
	}
	for i, result := range forks {
		if errs[i] != nil {
			t.Fatalf("fork %d: %v", i, errs[i])
		}
		child, err := metadata.Repos().GetByID(ctx, result.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertReadme(t, managers[i], child)
	}
}

func forkFixture(t *testing.T, ctx context.Context) (meta.V2Store, types.Namespace) {
	t.Helper()
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	metadata, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := metadata.(interface{ Close() }); ok {
		t.Cleanup(closer.Close)
	}
	if err := metadata.Accounts().Ensure(ctx, "acct"); err != nil {
		t.Fatal(err)
	}
	ns, err := metadata.Namespaces().Create(ctx, types.Namespace{AccountID: "acct", Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	return metadata, ns
}
