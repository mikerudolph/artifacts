package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mikerudolph/artifacts/internal/config"
)

type setupError struct{ message string }

func (e *setupError) Error() string     { return e.message }
func setupFailure(message string) error { return &setupError{message} }

func connectionConfig(ctx context.Context, cfg config.Postgres) (*pgxpool.Config, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pool, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, setupFailure("invalid database connection configuration")
	}
	conn := pool.ConnConfig
	if conn.ConnectTimeout == 0 {
		conn.ConnectTimeout = 10 * time.Second
	}
	if cfg.Schema != "" {
		path := pgx.Identifier{cfg.Schema}.Sanitize() + ",pg_temp"
		if previous := conn.RuntimeParams["search_path"]; previous != "" && previous != path && previous != cfg.Schema {
			return nil, setupFailure("database schema conflicts with connection search_path")
		}
		conn.RuntimeParams["search_path"] = path
		pool.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			return checkSelectedSchema(ctx, conn, cfg.Schema)
		}
	}
	if cfg.Auth == "rds-iam" {
		pool.BeforeConnect, err = databaseIAM(ctx, cfg, conn)
		if err != nil {
			return nil, err
		}
	}
	return pool, nil
}

func checkSelectedSchema(ctx context.Context, conn *pgx.Conn, schema string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var current string
	err := conn.QueryRow(ctx, "SELECT coalesce(current_schema(), '')").Scan(&current)
	if err != nil {
		return err
	}
	if current != schema {
		return setupFailure("selected database schema is missing or inaccessible; provision it with the migration identity")
	}
	return nil
}

func databaseError(err error) error {
	if err == nil {
		return nil
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			return fmt.Errorf("database operation interrupted: %w", cause)
		}
	}
	var setup *setupError
	if errors.As(err, &setup) {
		return setup
	}
	var server *pgconn.PgError
	if errors.As(err, &server) {
		return fmt.Errorf("database operation failed (SQLSTATE %s)", server.Code)
	}
	return setupFailure("database connection or migration failed; check connectivity, TLS, credentials, schema and migration permissions")
}
