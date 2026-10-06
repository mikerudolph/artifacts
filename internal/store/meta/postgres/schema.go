package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mikerudolph/artifacts/migrations"
)

func (s *store) CheckSchema(ctx context.Context) error {
	var version uint
	var dirty bool
	table := pgx.Identifier{s.schema, "schema_migrations"}.Sanitize()
	if err := s.q.QueryRow(ctx, "SELECT version, dirty FROM "+table).Scan(&version, &dirty); err != nil {
		return fmt.Errorf("database schema unavailable; run artifacts migrate: %w", databaseError(err))
	}
	if dirty || version != latestMigration() {
		return fmt.Errorf("database schema is not current; run artifacts migrate")
	}
	return nil
}

func latestMigration() uint {
	entries, _ := migrations.FS.ReadDir(".")
	var latest uint
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		prefix, _, _ := strings.Cut(entry.Name(), "_")
		version, err := strconv.ParseUint(prefix, 10, 32)
		if err == nil && uint(version) > latest {
			latest = uint(version)
		}
	}
	return latest
}
