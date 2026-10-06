package postgres

import (
	"context"
	"encoding/json"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *store) FindIdempotent(ctx context.Context, scope, key, digest string) (json.RawMessage, error) {
	var previous string
	var result json.RawMessage
	err := s.q.QueryRow(ctx, `SELECT digest, result FROM idempotency_results WHERE scope=$1 AND key=$2`, scope, key).Scan(&previous, &result)
	if err != nil {
		return nil, wrap(err)
	}
	if previous != digest {
		return nil, types.ErrIdempotencyConflict
	}
	return result, nil
}

func (s *store) RunIdempotent(ctx context.Context, scope, key, digest string, fn func(meta.Store) (json.RawMessage, error)) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.RunInTx(ctx, func(tx meta.Store) error {
		inner := tx.(*store)
		lock, _ := json.Marshal([]string{s.schema, scope, key})
		if _, err := inner.q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, string(lock)); err != nil {
			return err
		}
		var previous string
		err := inner.q.QueryRow(ctx, `SELECT digest, result FROM idempotency_results WHERE scope=$1 AND key=$2`, scope, key).Scan(&previous, &result)
		if err == nil {
			if previous != digest {
				return types.ErrIdempotencyConflict
			}
			return nil
		}
		if !meta.IsNotFound(wrap(err)) {
			return err
		}
		result, err = fn(inner)
		if err != nil {
			return err
		}
		_, err = inner.q.Exec(ctx, `INSERT INTO idempotency_results (scope,key,digest,result) VALUES ($1,$2,$3,$4)`, scope, key, digest, []byte(result))
		return err
	})
	return result, err
}
