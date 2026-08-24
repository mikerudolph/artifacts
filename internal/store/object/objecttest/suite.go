package objecttest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

// Factory constructs a Store for one test. The store must start empty.
type Factory func(tb testing.TB) object.Store

// Run exercises the Store contract. T3 must call this against fs and s3.
func Run(t *testing.T, newStore Factory) {
	t.Helper()
	t.Run("put-get", func(t *testing.T) { testPutGet(t, newStore) })
	t.Run("missing", func(t *testing.T) { testMissing(t, newStore) })
	t.Run("list-copy-delete", func(t *testing.T) { testListCopyDelete(t, newStore) })
	t.Run("invalid-key", func(t *testing.T) { testInvalidKey(t, newStore) })
}

func testPutGet(t *testing.T, newStore Factory) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	payload := []byte("hello artifacts")
	if err := s.Put(ctx, "acct/repo/objects/ab/cd", bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	ok, err := s.Exists(ctx, "acct/repo/objects/ab/cd")
	if err != nil || !ok {
		t.Fatalf("exists: %v %v", ok, err)
	}
	rc, err := s.Get(ctx, "acct/repo/objects/ab/cd")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
}

func testMissing(t *testing.T, newStore Factory) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	_, err := s.Get(ctx, "no/such")
	if !object.IsNotFound(err) {
		t.Fatalf("get: %v", err)
	}
	ok, err := s.Exists(ctx, "no/such")
	if err != nil || ok {
		t.Fatalf("exists: %v %v", ok, err)
	}
	if err := s.Delete(ctx, "no/such"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	if err := s.Copy(ctx, "no/such", "dst/x"); !object.IsNotFound(err) {
		t.Fatalf("copy missing: %v", err)
	}
}

func testListCopyDelete(t *testing.T, newStore Factory) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	mustPut(t, s, "acct/r1/a", "1")
	mustPut(t, s, "acct/r1/b", "2")
	mustPut(t, s, "acct/r2/c", "3")
	keys, err := s.List(ctx, "acct/r1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "acct/r1/a" || keys[1] != "acct/r1/b" {
		t.Fatalf("list %v", keys)
	}
	if err := s.Copy(ctx, "acct/r1/a", "acct/r3/a"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePrefix(ctx, "acct/r1/"); err != nil {
		t.Fatal(err)
	}
	keys, err = s.List(ctx, "acct/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("after delete prefix %v", keys)
	}
	if err := s.Delete(ctx, "acct/r2/c"); err != nil {
		t.Fatal(err)
	}
	ok, err := s.Exists(ctx, "acct/r2/c")
	if err != nil || ok {
		t.Fatalf("deleted still exists: %v %v", ok, err)
	}
}

func testInvalidKey(t *testing.T, newStore Factory) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	if err := s.Put(ctx, "../x", bytes.NewReader([]byte("z")), 1); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("put: %v", err)
	}
	if _, err := s.Get(ctx, "/abs"); !errors.Is(err, object.ErrInvalidKey) {
		t.Fatalf("get: %v", err)
	}
}

func mustPut(t *testing.T, s object.Store, key, val string) {
	t.Helper()
	if err := s.Put(context.Background(), key, bytes.NewReader([]byte(val)), int64(len(val))); err != nil {
		t.Fatal(err)
	}
}
