package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var databaseSchema = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

func loadDatabase() (Postgres, error) {
	cfg := Postgres{
		DSN:              firstEnv("DATABASE_URL", "ARTIFACTS_DATABASE_URL"),
		Auth:             env("ARTIFACTS_DATABASE_AUTH", "dsn"),
		Region:           env("ARTIFACTS_DATABASE_REGION", ""),
		Schema:           env("ARTIFACTS_DATABASE_SCHEMA", ""),
		MigrationTimeout: durationEnv("ARTIFACTS_DATABASE_MIGRATION_TIMEOUT", 15*time.Minute),
	}
	if cfg.MigrationTimeout <= 0 {
		return cfg, fmt.Errorf("ARTIFACTS_DATABASE_MIGRATION_TIMEOUT must be positive")
	}
	return cfg, cfg.Validate()
}

func (p Postgres) Validate() error {
	if p.Auth != "" && p.Auth != "dsn" && p.Auth != "rds-iam" {
		return fmt.Errorf("ARTIFACTS_DATABASE_AUTH must be dsn or rds-iam")
	}
	if p.Schema != "" && (!databaseSchema.MatchString(p.Schema) || strings.HasPrefix(p.Schema, "pg_") || p.Schema == "information_schema") {
		return fmt.Errorf("ARTIFACTS_DATABASE_SCHEMA must be a lowercase identifier of 1 to 63 bytes outside system schemas")
	}
	if p.MigrationTimeout < 0 || p.MigrationTimeout > time.Hour {
		return fmt.Errorf("database migration timeout must be positive and at most one hour")
	}
	return nil
}
