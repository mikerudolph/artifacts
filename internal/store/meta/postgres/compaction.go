package postgres

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *store) RunCompaction(ctx context.Context, id types.RepoID, fn func(meta.V2Store) error) error {
	return s.RunInTx(ctx, func(tx meta.Store) error {
		inner := tx.(*store)
		var acquired bool
		if err := inner.q.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))", "compaction/"+string(id)).Scan(&acquired); err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		return fn(inner)
	})
}
