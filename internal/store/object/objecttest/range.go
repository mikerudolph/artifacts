package objecttest

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

func testRange(t *testing.T, factory Factory) {
	t.Helper()
	ctx := context.Background()
	s := factory(t)
	ranges, ok := s.(object.RangeStore)
	if !ok {
		t.Fatal("backend does not support ranges")
	}
	if err := s.Put(ctx, "range/file", strings.NewReader("0123456789"), 10); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		offset, length int64
		want           string
	}{{0, 3, "012"}, {4, 3, "456"}, {8, 10, "89"}} {
		r, err := ranges.GetRange(ctx, "range/file", tc.offset, tc.length)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil || string(b) != tc.want {
			t.Fatalf("range: %q %v", b, err)
		}
	}
	if _, err := ranges.GetRange(ctx, "range/file", 10, 5); !errors.Is(err, io.EOF) {
		t.Fatalf("EOF: %v", err)
	}
	if _, err := ranges.GetRange(ctx, "range/missing", 0, 5); !object.IsNotFound(err) {
		t.Fatalf("missing: %v", err)
	}
	for _, tc := range []struct {
		key            string
		offset, length int64
	}{{"../bad", 0, 1}, {"range/file", -1, 1}, {"range/file", 0, 0}} {
		if _, err := ranges.GetRange(ctx, tc.key, tc.offset, tc.length); err == nil {
			t.Fatal("invalid range accepted")
		}
	}
}
