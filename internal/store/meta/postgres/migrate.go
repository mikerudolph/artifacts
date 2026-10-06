package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/migrations"
)

func Migrate(dsn string) error {
	return MigrateConfigured(context.Background(), config.Postgres{DSN: dsn}, false)
}

func MigrateConfigured(ctx context.Context, cfg config.Postgres, createSchema bool) error {
	if createSchema && cfg.Schema == "" {
		return setupFailure("--create-schema requires ARTIFACTS_DATABASE_SCHEMA")
	}
	limit := cfg.MigrationTimeout
	if limit == 0 {
		limit = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	settings, err := connectionConfig(ctx, cfg)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var stops []func() bool
	after := func(_ context.Context, conn *pgx.Conn) error {
		stop := context.AfterFunc(ctx, func() { _ = conn.PgConn().Conn().Close() })
		mu.Lock()
		stops = append(stops, stop)
		mu.Unlock()
		if cfg.Schema != "" {
			return prepareSchema(ctx, conn, cfg.Schema, createSchema)
		}
		return ctx.Err()
	}
	connector := stdlib.GetConnector(*settings.ConnConfig, stdlib.OptionBeforeConnect(connectionHook(settings.BeforeConnect)), stdlib.OptionAfterConnect(after))
	db := sql.OpenDB(migrationConnector{Connector: connector, ctx: ctx})
	db.SetMaxOpenConns(1)
	defer func() {
		_ = db.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, stop := range stops {
			stop()
		}
	}()
	err = migrateDatabase(db, cfg.Schema)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return databaseError(err)
}

type migrationConnector struct {
	driver.Connector
	ctx context.Context
}

func (c migrationConnector) Connect(ctx context.Context) (driver.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	defer cancel()
	if c.ctx.Err() != nil {
		return nil, c.ctx.Err()
	}
	return c.Connector.Connect(ctx)
}

func connectionHook(hook func(context.Context, *pgx.ConnConfig) error) func(context.Context, *pgx.ConnConfig) error {
	return func(ctx context.Context, conn *pgx.ConnConfig) error {
		if hook != nil {
			return hook(ctx, conn)
		}
		return ctx.Err()
	}
}

func migrateDatabase(db *sql.DB, schema string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{SchemaName: schema})
	if err != nil {
		return err
	}
	defer func() { _ = driver.Close() }()
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return setupFailure("database migration failed; inspect the selected schema's migration version and permissions before retrying")
	}
	return nil
}
