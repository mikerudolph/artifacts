package config

import (
	"testing"
	"time"
)

func TestDatabaseConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "database")
	t.Setenv("ARTIFACTS_DATABASE_SCHEMA", "artifacts")
	t.Setenv("ARTIFACTS_DATABASE_AUTH", "rds-iam")
	t.Setenv("ARTIFACTS_DATABASE_REGION", "us-west-2")
	cfg, err := LoadDatabase()
	if err != nil || cfg.Schema != "artifacts" || cfg.Auth != "rds-iam" || cfg.Region != "us-west-2" || cfg.MigrationTimeout != 15*time.Minute {
		t.Fatal("database configuration", err)
	}
	for _, schema := range []string{"pg_private", "information_schema", "x,public", "x\"", "Upper", "a.b"} {
		t.Setenv("ARTIFACTS_DATABASE_SCHEMA", schema)
		if _, err := LoadDatabase(); err == nil {
			t.Fatal("accepted invalid schema")
		}
	}
	t.Setenv("ARTIFACTS_DATABASE_SCHEMA", "")
	t.Setenv("ARTIFACTS_DATABASE_AUTH", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("accepted invalid database auth")
	}
	t.Setenv("ARTIFACTS_DATABASE_AUTH", "dsn")
	for _, timeout := range []string{"invalid", "0s", "2h"} {
		t.Setenv("ARTIFACTS_DATABASE_MIGRATION_TIMEOUT", timeout)
		if _, err := LoadDatabase(); err == nil {
			t.Fatal("accepted invalid migration timeout")
		}
	}
	t.Setenv("ARTIFACTS_DATABASE_MIGRATION_TIMEOUT", "1m")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ARTIFACTS_DATABASE_URL", "")
	if _, err := LoadDatabase(); err == nil {
		t.Fatal("accepted missing database")
	}
}
