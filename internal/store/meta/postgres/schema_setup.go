package postgres

import (
	"context"
	"fmt"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/jackc/pgx/v5"
)

func prepareSchema(ctx context.Context, conn *pgx.Conn, schema string, create bool) error {
	var name string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		return err
	}
	id, err := database.GenerateAdvisoryLockId(name, schema, "schema_migrations")
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", id); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", id) }()
	if create {
		if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{schema}.Sanitize()); err != nil {
			return err
		}
	}
	if err := checkSelectedSchema(ctx, conn, schema); err != nil {
		return err
	}
	return inspectInstallation(ctx, conn, schema)
}

var installationTables = map[string][]string{
	"accounts":            {"id", "created_at"},
	"namespaces":          {"id", "account_id", "name", "jurisdiction", "created_at", "updated_at"},
	"repos":               {"id", "namespace_id", "name", "description", "default_branch", "read_only", "source", "status", "created_at", "updated_at", "last_push_at"},
	"refs":                {"repo_id", "name", "sha"},
	"repo_tokens":         {"id", "repo_id", "hash", "scope", "state", "created_at", "expires_at"},
	"api_tokens":          {"id", "account_id", "hash", "created_at"},
	"jobs":                {"id", "repo_id", "kind", "status", "error", "progress", "created_at", "updated_at"},
	"pack_wal":            {"repo_id", "sequence", "pack_key", "index_key", "checksum", "size", "created_at"},
	"pack_ref_updates":    {"repo_id", "sequence", "name", "old_sha", "new_sha"},
	"checkpoints":         {"repo_id", "sequence", "pack_key", "index_key", "checksum", "created_at"},
	"repo_forks":          {"repo_id", "parent_repo_id", "parent_sequence", "created_at"},
	"idempotency_results": {"scope", "key", "digest", "result", "created_at"},
	"schema_migrations":   {"version", "dirty"},
}

func inspectInstallation(ctx context.Context, conn *pgx.Conn, schema string) error {
	var extra bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1)
OR EXISTS(SELECT 1 FROM pg_catalog.pg_type t JOIN pg_catalog.pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname=$1 AND t.typrelid=0 AND t.typelem=0)`, schema).Scan(&extra); err != nil {
		return err
	}
	if extra {
		return setupFailure("selected schema contains unrelated functions or types; use a dedicated empty schema")
	}
	tables, err := installationRelations(ctx, conn, schema)
	if err != nil || len(tables) == 0 {
		return err
	}
	if !tables["schema_migrations"] {
		return setupFailure("selected schema is populated without Artifacts migration history")
	}
	for name := range tables {
		if err := inspectColumns(ctx, conn, schema, name); err != nil {
			return err
		}
	}
	return inspectHistory(ctx, conn, schema, tables)
}

func installationRelations(ctx context.Context, conn *pgx.Conn, schema string) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT c.relname, c.relkind::text FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname=$1 AND c.relkind NOT IN ('i','I','t')`, schema)
	if err != nil {
		return nil, err
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if kind != "r" || installationTables[name] == nil {
			rows.Close()
			return nil, setupFailure("selected schema contains unrelated objects; use a dedicated empty schema")
		}
		tables[name] = true
	}
	rows.Close()
	return tables, rows.Err()
}

func inspectHistory(ctx context.Context, conn *pgx.Conn, schema string, tables map[string]bool) error {
	var version uint64
	var dirty bool
	err := conn.QueryRow(ctx, "SELECT version, dirty FROM "+pgx.Identifier{schema, "schema_migrations"}.Sanitize()).Scan(&version, &dirty)
	if err == pgx.ErrNoRows && len(tables) == 1 {
		return nil
	}
	if err != nil || dirty || version < 1 || version > uint64(latestMigration()) {
		return setupFailure("selected schema has incomplete, dirty, or unsupported migration history")
	}
	introduced := map[string]uint64{"pack_wal": 2, "pack_ref_updates": 2, "checkpoints": 2, "repo_forks": 2, "idempotency_results": 3}
	for table := range installationTables {
		if tables[table] != (version >= introduced[table]) {
			return setupFailure("selected schema does not match its Artifacts migration history")
		}
	}
	return nil
}

func inspectColumns(ctx context.Context, conn *pgx.Conn, schema, table string) error {
	var count int
	err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_attribute a
JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname=$1 AND c.relname=$2 AND a.attname=ANY($3) AND a.attnum>0 AND NOT a.attisdropped`, schema, table, installationTables[table]).Scan(&count)
	if err != nil {
		return err
	}
	if count != len(installationTables[table]) {
		return setupFailure(fmt.Sprintf("selected schema has an incompatible %s table", table))
	}
	return nil
}
