package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
)

func TestNewEmptyRoot(t *testing.T) {
	t.Parallel()
	if _, err := New(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewOnFile(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "notdir")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(p); err == nil {
		t.Fatal("expected error")
	}
}

func TestInvalidKeyOps(t *testing.T) {
	t.Parallel()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Delete(ctx, "../x"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Exists(ctx, "../x"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("exists: %v", err)
	}
}

func TestConformance(t *testing.T) {
	t.Parallel()
	objecttest.Run(t, func(tb testing.TB) object.Store {
		s, err := New(tb.TempDir())
		if err != nil {
			tb.Fatal(err)
		}
		return s
	})
}
