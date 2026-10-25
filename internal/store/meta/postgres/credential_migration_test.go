package postgres

import (
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/migrations"
)

func TestCredentialRetentionUpgrade(t *testing.T) {
	for _, schema := range []string{"", "retention"} {
		t.Run("schema="+schema, func(t *testing.T) {
			dsn, admin := schemaAdmin(t)
			cfg := config.Postgres{DSN: dsn, Schema: schema}
			if schema != "" {
				execSchema(t, admin, "CREATE SCHEMA retention; SET search_path=retention,pg_temp")
			}
			for _, file := range []string{"001_init.up.sql", "002_v2_storage.up.sql", "003_idempotency.up.sql"} {
				body, err := migrations.FS.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				execSchema(t, admin, string(body))
			}
			execSchema(t, admin, `CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean);
				INSERT INTO schema_migrations VALUES(3,false);
				INSERT INTO accounts(id) VALUES('local');
				INSERT INTO namespaces(id,account_id,name) VALUES('ns','local','default');
				INSERT INTO repos(id,namespace_id,account_id,name) VALUES('repo','ns','local','app');
				INSERT INTO repo_tokens(id,repo_id,hash,scope,state,expires_at) VALUES
				('old','repo','old','read','revoked',now()-interval '10 days'),
				('revoked','repo','revoked','read','revoked',now()+interval '1 day'),
				('active','repo','active','read','active',now()+interval '1 day')`)
			before := time.Now().Add(-time.Second)
			if err := MigrateConfigured(t.Context(), cfg, false); err != nil {
				t.Fatal(err)
			}
			metadata, err := OpenConfigured(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			s := metadata.(*store)
			defer s.Close()
			first := revokedAt(t, s, "revoked")
			if first.Before(before) || first.After(time.Now()) {
				t.Fatal("legacy revocation time was invented")
			}
			if err := MigrateConfigured(t.Context(), cfg, false); err != nil {
				t.Fatal(err)
			}
			if !first.Equal(revokedAt(t, s, "revoked")) {
				t.Fatal("repeated migration changed timestamp")
			}
			if n, err := s.RepoTokens().Prune(t.Context(), time.Now().Add(-7*24*time.Hour)); n != 1 || err != nil {
				t.Fatalf("legacy cleanup: %d %v", n, err)
			}
			checkCredentialMigrationShape(t, cfg, s, schema)
		})
	}
}

func checkCredentialMigrationShape(t *testing.T, cfg config.Postgres, s *store, schema string) {
	t.Helper()
	if err := s.CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if schema == "" {
		down, err := migrations.FS.ReadFile("004_credential_retention.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		retentionExec(t, s, string(down))
		retentionExec(t, s, "UPDATE schema_migrations SET version=3")
		if err := MigrateConfigured(t.Context(), cfg, false); err != nil {
			t.Fatal(err)
		}
		if err := s.CheckSchema(t.Context()); err != nil {
			t.Fatal(err)
		}
		return
	}
	retentionExec(t, s, "ALTER TABLE repo_tokens DROP COLUMN revoked_at CASCADE")
	if err := MigrateConfigured(t.Context(), cfg, false); err == nil {
		t.Fatal("accepted current schema without revocation timestamp")
	}
}
