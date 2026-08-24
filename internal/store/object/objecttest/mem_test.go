package objecttest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

func TestMemDeletePrefixEmptyAndInvalid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMem()
	if err := m.Put(ctx, "a/b", bytes.NewReader([]byte("x")), 1); err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePrefix(ctx, "../x"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("invalid: %v", err)
	}
	if err := m.DeletePrefix(ctx, ""); err != nil {
		t.Fatal(err)
	}
	keys, err := m.List(ctx, "")
	if err != nil || len(keys) != 0 {
		t.Fatalf("list after empty prefix delete: %v %v", keys, err)
	}
}

func TestMemInvalidKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMem()
	if err := m.Delete(ctx, "../x"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("delete: %v", err)
	}
	if err := m.Copy(ctx, "../x", "ok"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("copy src: %v", err)
	}
	if err := m.Copy(ctx, "ok", "../x"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("copy dst: %v", err)
	}
	if _, err := m.Exists(ctx, "../x"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("exists: %v", err)
	}
}

func TestMemPutReadError(t *testing.T) {
	t.Parallel()
	m := NewMem()
	err := m.Put(context.Background(), "k", errReader{}, 1)
	if err == nil {
		t.Fatal("expected read error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
