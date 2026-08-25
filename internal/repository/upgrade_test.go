package repository

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/migrations"
)

func TestStorageV1MigrationConvertsAndRebuilds(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dsn := testkit.Postgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	execMigration(t, db, "001_init.up.sql")
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id) VALUES('acct');
		INSERT INTO namespaces(id,account_id,name) VALUES('ns','acct','default');
		INSERT INTO repos(id,namespace_id,name,default_branch,status) VALUES('legacy','ns','legacy','main','ready')`); err != nil {
		t.Fatal(err)
	}
	execMigration(t, db, "002_v2_storage.up.sql")
	metadata, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closer, ok := metadata.(interface{ Close() }); ok {
			closer.Close()
		}
	}()
	objects := objecttest.NewMem()
	writeLegacyRepository(t, ctx, metadata, objects)
	repo, err := metadata.Repos().GetByID(ctx, "legacy")
	if err != nil || repo.StorageVersion != 1 {
		t.Fatalf("legacy repo %+v %v", repo, err)
	}
	manager, err := New(metadata, objects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assertReadme(t, manager, repo)
	upgraded, err := metadata.Repos().GetByID(ctx, repo.ID)
	if err != nil || upgraded.StorageVersion != 2 || upgraded.WALSequence != 1 {
		t.Fatalf("upgraded repo %+v %v", upgraded, err)
	}
	if err := manager.Evict(upgraded); err != nil {
		t.Fatal(err)
	}
	assertReadme(t, manager, upgraded)
}

func execMigration(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	contents, err := migrations.FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(contents)); err != nil {
		t.Fatalf("migration %s: %v", name, err)
	}
}

func writeLegacyRepository(t *testing.T, _ context.Context, metadata meta.V2Store, objects *objecttest.Mem) {
	t.Helper()
	source := testkit.TempRepo(t)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.RunGit(t, source, "-c", "core.hooksPath="+t.TempDir(), "-c", "commit.gpgsign=false", "add", "README.md")
	testkit.RunGit(t, source, "-c", "core.hooksPath="+t.TempDir(), "-c", "commit.gpgsign=false", "commit", "-m", "legacy")
	src, err := gogit.PlainOpen(source)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := gitstore.Open(objects, metadata.Refs(), "acct", "legacy")
	if err != nil {
		t.Fatal(err)
	}
	iter, err := src.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
		_, err := dst.SetEncodedObject(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	refs, err := src.Storer.IterReferences()
	if err != nil {
		t.Fatal(err)
	}
	defer refs.Close()
	if err := refs.ForEach(dst.SetReference); err != nil {
		t.Fatal(err)
	}
}
