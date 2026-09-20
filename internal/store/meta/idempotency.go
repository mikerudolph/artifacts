package meta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

type IdempotentStore interface {
	RunIdempotent(context.Context, string, string, string, func(Store) (json.RawMessage, error)) (json.RawMessage, error)
}

func Idempotent[T any](ctx context.Context, st Store, scope, key string, input any, fn func(Store) (T, error)) (T, error) {
	var zero T
	if len(key) > 128 || strings.TrimSpace(key) != key || key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return zero, &types.InputError{Field: "Idempotency-Key", Message: "idempotency key must contain 1 to 128 bytes without surrounding whitespace or control characters"}
	}
	store, ok := st.(IdempotentStore)
	if !ok {
		return zero, errors.New("durable idempotency unavailable")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return zero, err
	}
	digest := sha256.Sum256(body)
	result, err := store.RunIdempotent(ctx, scope, key, hex.EncodeToString(digest[:]), func(tx Store) (json.RawMessage, error) {
		value, err := fn(tx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	})
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(result, &zero)
	return zero, err
}
