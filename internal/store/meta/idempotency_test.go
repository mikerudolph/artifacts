package meta

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/types"
)

func TestInvalidIdempotencyKeysCannotExecuteMutation(t *testing.T) {
	mutated := false
	mutation := func(Store) (string, error) { mutated = true; return "changed", nil }
	for _, key := range []string{"", " key", "key ", "key\nvalue", "key\x00value", strings.Repeat("x", 129)} {
		_, err := Idempotent(context.Background(), nil, "scope", key, "body", mutation)
		var input *types.InputError
		if !errors.As(err, &input) || input.Field != "Idempotency-Key" || input.Error() == "" {
			t.Fatalf("invalid key error: %v", err)
		}
	}
	if mutated {
		t.Fatal("invalid key executed mutation")
	}
	if _, err := Idempotent(context.Background(), nil, "scope", "valid", "body", mutation); err == nil {
		t.Fatal("silently accepted nondurable store")
	}
	if mutated {
		t.Fatal("unsupported store executed mutation")
	}
}
