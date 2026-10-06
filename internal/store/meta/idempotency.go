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

type IdempotentReader interface {
	FindIdempotent(context.Context, string, string, string) (json.RawMessage, error)
}

func ValidateIdempotencyKey(key string) error {
	if len(key) > 128 || strings.TrimSpace(key) != key || key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return &types.InputError{Field: "Idempotency-Key", Message: "idempotency key must contain 1 to 128 bytes without surrounding whitespace or control characters"}
	}
	return nil
}

func RequestDigest(input any) (string, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func Idempotent[T any](ctx context.Context, st Store, scope, key string, input any, fn func(Store) (T, error)) (T, error) {
	digest, err := RequestDigest(input)
	if err != nil {
		var zero T
		return zero, err
	}
	return IdempotentDigest(ctx, st, scope, key, digest, fn)
}

func IdempotentDigest[T any](ctx context.Context, st Store, scope, key, digest string, fn func(Store) (T, error)) (T, error) {
	var zero T
	if err := ValidateIdempotencyKey(key); err != nil {
		return zero, err
	}
	store, ok := st.(IdempotentStore)
	if !ok {
		return zero, errors.New("durable idempotency unavailable")
	}
	result, err := store.RunIdempotent(ctx, scope, key, digest, func(tx Store) (json.RawMessage, error) {
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
