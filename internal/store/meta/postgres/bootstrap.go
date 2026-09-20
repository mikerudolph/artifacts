package postgres

import (
	"context"
	"fmt"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *store) EnsureAPIToken(ctx context.Context, account types.AccountID, hash string) error {
	return s.RunInTx(ctx, func(tx meta.Store) error {
		inner := tx.(*store)
		if err := inner.Accounts().Ensure(ctx, account); err != nil {
			return err
		}
		var id string
		err := inner.q.QueryRow(ctx, `INSERT INTO api_tokens (id, account_id, hash)
   VALUES ($1,$2,$3) ON CONFLICT (hash) DO UPDATE SET hash=excluded.hash
   WHERE api_tokens.account_id=excluded.account_id RETURNING id`, newID(), account, hash).Scan(&id)
		if meta.IsNotFound(wrap(err)) {
			return fmt.Errorf("control token is already bound to another account")
		}
		return err
	})
}
