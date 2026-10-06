package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/jackc/pgx/v5"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
	"github.com/mikerudolph/artifacts/migrations"
)

func schemaAdmin(t *testing.T) (string, *pgx.Conn) {
	t.Helper()
	dsn := testkit.Postgres(t)
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return dsn, conn
}

func execSchema(t *testing.T, conn *pgx.Conn, query string) {
	t.Helper()
	if _, err := conn.Exec(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}

func TestNamedSchemaIsolationAndPermissions(t *testing.T) {
	dsn, admin := schemaAdmin(t)
	execSchema(t, admin, `CREATE TABLE public.accounts(sentinel text); INSERT INTO public.accounts VALUES ('untouched');
CREATE TABLE public.schema_migrations(version bigint, dirty boolean); INSERT INTO public.schema_migrations VALUES (999,false);
CREATE ROLE schema_owner LOGIN PASSWORD 'local-test'; CREATE ROLE schema_runtime LOGIN PASSWORD 'local-test';
CREATE SCHEMA artifacts AUTHORIZATION schema_owner`)
	owner, _ := url.Parse(dsn)
	owner.User = url.UserPassword("schema_owner", "local-test")
	cfg := config.Postgres{DSN: owner.String(), Schema: "artifacts"}
	for range 2 {
		if err := MigrateConfigured(t.Context(), cfg, false); err != nil {
			t.Fatal(err)
		}
	}
	execSchema(t, admin, `GRANT USAGE ON SCHEMA artifacts TO schema_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA artifacts TO schema_runtime`)
	owner.User = url.UserPassword("schema_runtime", "local-test")
	cfg.DSN = owner.String()
	metadata, err := OpenConfigured(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := metadata.(*store)
	defer s.Close()
	if err := s.CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Accounts().Ensure(t.Context(), "tenant"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"CREATE TABLE artifacts.forbidden(id int)", "UPDATE public.accounts SET sentinel='changed'"} {
		if _, err := s.q.Exec(t.Context(), query); err == nil {
			t.Fatal("runtime permission boundary failed")
		}
	}
	var sentinel string
	var version int
	if err := admin.QueryRow(t.Context(), "SELECT sentinel FROM public.accounts").Scan(&sentinel); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(t.Context(), "SELECT version FROM public.schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if sentinel != "untouched" || version != 999 {
		t.Fatal("neighboring application changed")
	}
	cfg.Schema = "missing"
	if _, err := OpenConfigured(t.Context(), cfg); err == nil {
		t.Fatal("missing schema fell back")
	}
	execSchema(t, admin, "CREATE SCHEMA private")
	cfg.Schema = "private"
	if _, err := OpenConfigured(t.Context(), cfg); err == nil {
		t.Fatal("inaccessible schema accepted")
	}
}

func TestNamedSchemaConcurrentMigrationsAndLegacyUpgrade(t *testing.T) {
	dsn, admin := schemaAdmin(t)
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for _, schema := range []string{"one", "one", "two", "two"} {
		wg.Go(func() { errors <- MigrateConfigured(t.Context(), config.Postgres{DSN: dsn, Schema: schema}, true) })
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, schema := range []string{"one", "two"} {
		st, err := OpenConfigured(t.Context(), config.Postgres{DSN: dsn, Schema: schema})
		if err != nil {
			t.Fatal(err)
		}
		s := st.(*store)
		if err := s.CheckSchema(t.Context()); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
	execSchema(t, admin, "CREATE SCHEMA upgrade; SET search_path=upgrade,pg_temp")
	initial, err := migrations.FS.ReadFile("001_init.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	execSchema(t, admin, string(initial))
	execSchema(t, admin, "CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean); INSERT INTO schema_migrations VALUES (1,false)")
	if err := MigrateConfigured(t.Context(), config.Postgres{DSN: dsn, Schema: "upgrade"}, false); err != nil {
		t.Fatal(err)
	}
}

func TestNamedSchemaRejectsUnrelatedInstallations(t *testing.T) {
	dsn, admin := schemaAdmin(t)
	for _, objects := range []string{
		"CREATE TABLE unrelated(id int)",
		"CREATE FUNCTION unrelated() RETURNS int LANGUAGE sql AS 'SELECT 1'",
		"CREATE TYPE unrelated AS ENUM ('value')",
		"CREATE TABLE accounts(id text,created_at timestamptz)",
		"CREATE TABLE schema_migrations(version bigint,dirty boolean); INSERT INTO schema_migrations VALUES (3,false)",
		"CREATE TABLE schema_migrations(unknown text)",
		"CREATE TABLE schema_migrations(version bigint,dirty boolean); INSERT INTO schema_migrations VALUES (3,true)",
	} {
		execSchema(t, admin, "CREATE SCHEMA candidate; SET search_path=candidate,pg_temp")
		execSchema(t, admin, objects)
		if err := MigrateConfigured(t.Context(), config.Postgres{DSN: dsn, Schema: "candidate"}, false); err == nil {
			t.Fatal("unrelated schema adopted")
		}
		execSchema(t, admin, "DROP SCHEMA candidate CASCADE")
	}
	if err := MigrateConfigured(t.Context(), config.Postgres{DSN: dsn}, true); err == nil {
		t.Fatal("schema creation without selection")
	}
}

func TestMigrationTimeoutDuringHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cfg := config.Postgres{DSN: "postgres://user@" + listener.Addr().String() + "/db?sslmode=disable", MigrationTimeout: 100 * time.Millisecond}
	start := time.Now()
	if err := MigrateConfigured(ctx, cfg, false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("handshake deadline", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("handshake exceeded migration deadline")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("connection was not closed")
	}
}

func TestMigrationCancellationReleasesLock(t *testing.T) {
	dsn, admin := schemaAdmin(t)
	var name string
	if err := admin.QueryRow(t.Context(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	id, err := database.GenerateAdvisoryLockId(name, "blocked", "schema_migrations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), "SELECT pg_advisory_lock($1)", id); err != nil {
		t.Fatal(err)
	}
	cfg := config.Postgres{DSN: dsn, Schema: "blocked", MigrationTimeout: 100 * time.Millisecond}
	if err := MigrateConfigured(t.Context(), cfg, true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("migration deadline: %v", err)
	}
	if _, err := admin.Exec(t.Context(), "SELECT pg_advisory_unlock($1)", id); err != nil {
		t.Fatal(err)
	}
	cfg.MigrationTimeout = time.Minute
	if err := MigrateConfigured(t.Context(), cfg, true); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaCompactionLocksAreIndependent(t *testing.T) {
	dsn, _ := schemaAdmin(t)
	stores := []*store{}
	for _, schema := range []string{"one", "two"} {
		cfg := config.Postgres{DSN: dsn, Schema: schema}
		if err := MigrateConfigured(t.Context(), cfg, true); err != nil {
			t.Fatal(err)
		}
		st, err := OpenConfigured(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		s := st.(*store)
		stores = append(stores, s)
		t.Cleanup(s.Close)
	}
	err := stores[0].RunInTx(t.Context(), func(st meta.Store) error {
		inner := st.(*store)
		if _, err := inner.q.Exec(t.Context(), "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "one/compaction/same"); err != nil {
			return err
		}
		called := false
		if err := stores[1].RunCompaction(t.Context(), types.RepoID("same"), func(meta.V2Store) error { called = true; return nil }); err != nil {
			return err
		}
		if !called {
			t.Fatal("different schemas share compaction lock")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaIdempotency(t, stores)
}

func assertSchemaIdempotency(t *testing.T, stores []*store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err := stores[0].RunIdempotent(ctx, "scope", "same-key", "digest", func(meta.Store) (json.RawMessage, error) {
		_, err := stores[1].RunIdempotent(ctx, "scope", "same-key", "digest", func(meta.Store) (json.RawMessage, error) {
			return json.RawMessage(`{"schema":"two"}`), nil
		})
		return json.RawMessage(`{"schema":"one"}`), err
	})
	if err != nil {
		t.Fatal("independent schema idempotency", err)
	}
	for i, s := range stores {
		result, err := s.FindIdempotent(ctx, "scope", "same-key", "digest")
		if err != nil {
			t.Fatal(err)
		}
		var value struct{ Schema string }
		if err := json.Unmarshal(result, &value); err != nil {
			t.Fatal(err)
		}
		if value.Schema != []string{"one", "two"}[i] {
			t.Fatal("idempotency crossed schemas")
		}
	}
}
