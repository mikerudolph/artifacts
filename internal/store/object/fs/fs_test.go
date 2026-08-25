package fs

import (
	"bytes"
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

func TestImmutablePut(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "account/repo/pack/one.pack"
	if err := store.Put(ctx, key, bytes.NewBufferString("one"), 3); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, key, bytes.NewBufferString("one"), 3); err != nil {
		t.Fatalf("idempotent put: %v", err)
	}
	if err := store.Put(ctx, key, bytes.NewBufferString("two"), 3); !errors.Is(err, object.ErrImmutableConflict) {
		t.Fatalf("conflict: %v", err)
	}
	if err := store.Put(ctx, "account/repo/fail", failingReader{}, -1); err == nil {
		t.Fatal("reader failure ignored")
	}
	if err := store.Copy(ctx, key, "../bad"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("copy destination: %v", err)
	}
}

func TestSameFileContentMissing(t *testing.T) {
	t.Parallel()
	if sameFileContent(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "also-missing")) {
		t.Fatal("missing files matched")
	}
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sameFileContent(existing, filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("existing file matched missing file")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

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
