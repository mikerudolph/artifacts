package postgres

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/types"
)

func (s accountStore) Ensure(ctx context.Context, id types.AccountID) error {
	_, err := s.q.Exec(ctx, `INSERT INTO accounts (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, string(id))
	return err
}

func (s accountStore) Get(ctx context.Context, id types.AccountID) (types.AccountID, error) {
	var out string
	err := s.q.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1`, string(id)).Scan(&out)
	if err != nil {
		return "", wrap(err)
	}
	return types.AccountID(out), nil
}
