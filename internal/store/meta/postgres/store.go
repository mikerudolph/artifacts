package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mikerudolph/artifacts/internal/store/meta"
)

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type store struct {
	q querier
	p *pgxpool.Pool
}

// Open connects to Postgres. Call Migrate first.
func Open(ctx context.Context, dsn string) (meta.Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &store{q: pool, p: pool}, nil
}

// Close releases the pool. Safe on a transaction-bound store.
func (s *store) Close() {
	if s.p != nil {
		s.p.Close()
	}
}

func (s *store) Accounts() meta.Accounts     { return accountStore{s} }
func (s *store) Namespaces() meta.Namespaces { return namespaceStore{s} }
func (s *store) Repos() meta.Repos           { return repoStore{s} }
func (s *store) Refs() meta.Refs             { return refStore{s} }
func (s *store) RepoTokens() meta.RepoTokens { return repoTokenStore{s} }
func (s *store) APITokens() meta.APITokens   { return apiTokenStore{s} }
func (s *store) Jobs() meta.Jobs             { return jobStore{s} }

type accountStore struct{ *store }
type namespaceStore struct{ *store }
type repoStore struct{ *store }
type refStore struct{ *store }
type repoTokenStore struct{ *store }
type apiTokenStore struct{ *store }
type jobStore struct{ *store }

func (s *store) RunInTx(ctx context.Context, fn func(meta.Store) error) error {
	if s.p == nil {
		return fn(s)
	}
	tx, err := s.p.Begin(ctx)
	if err != nil {
		return err
	}
	inner := &store{q: tx}
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
