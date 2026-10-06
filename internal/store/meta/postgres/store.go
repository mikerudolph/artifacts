package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta"
)

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type store struct {
	q      querier
	p      *pgxpool.Pool
	schema string
}

func Open(ctx context.Context, dsn string) (meta.V2Store, error) {
	return OpenConfigured(ctx, config.Postgres{DSN: dsn})
}

func OpenConfigured(ctx context.Context, cfg config.Postgres) (meta.V2Store, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	settings, err := connectionConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, settings)
	if err != nil {
		return nil, databaseError(err)
	}
	var schema string
	if err := pool.QueryRow(ctx, "SELECT coalesce(current_schema(), '')").Scan(&schema); err != nil {
		pool.Close()
		return nil, databaseError(err)
	}
	return &store{q: pool, p: pool, schema: schema}, nil
}

func (s *store) Close() {
	if s.p != nil {
		s.p.Close()
	}
}

func (s *store) Accounts() meta.Accounts       { return accountStore{s} }
func (s *store) Namespaces() meta.Namespaces   { return namespaceStore{s} }
func (s *store) Repos() meta.Repos             { return repoStore{s} }
func (s *store) Refs() meta.Refs               { return refStore{s} }
func (s *store) RepoTokens() meta.RepoTokens   { return repoTokenStore{s} }
func (s *store) APITokens() meta.APITokens     { return apiTokenStore{s} }
func (s *store) Jobs() meta.Jobs               { return jobStore{s} }
func (s *store) WAL() meta.WAL                 { return walStore{s} }
func (s *store) Checkpoints() meta.Checkpoints { return checkpointStore{s} }
func (s *store) Forks() meta.Forks             { return forkStore{s} }

type accountStore struct{ *store }
type namespaceStore struct{ *store }
type repoStore struct{ *store }
type refStore struct{ *store }
type repoTokenStore struct{ *store }
type apiTokenStore struct{ *store }
type jobStore struct{ *store }
type walStore struct{ *store }
type checkpointStore struct{ *store }
type forkStore struct{ *store }

func (s *store) RunInTx(ctx context.Context, fn func(meta.Store) error) error {
	if s.p == nil {
		return fn(s)
	}
	tx, err := s.p.Begin(ctx)
	if err != nil {
		return err
	}
	inner := &store{q: tx, schema: s.schema}
	if err := fn(inner); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return meta.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return meta.ErrAlreadyExists
	}
	return err
}
