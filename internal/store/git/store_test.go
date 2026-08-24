package gitstore

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage"
	objstore "github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
)

func TestObjectErrors(t *testing.T) {
	t.Parallel()
	st := mustStore(t)
	missing := plumbing.NewHash("0123456789abcdef0123456789abcdef01234567")
	if _, err := st.EncodedObject(plumbing.AnyObject, missing); err != plumbing.ErrObjectNotFound {
		t.Fatalf("get: %v", err)
	}
	if _, err := st.EncodedObjectSize(missing); err != plumbing.ErrObjectNotFound {
		t.Fatalf("size: %v", err)
	}
	bad := &plumbing.MemoryObject{}
	bad.SetType(plumbing.InvalidObject)
	if _, err := st.SetEncodedObject(bad); err == nil {
		t.Fatal("expected invalid type")
	}
	ref := plumbing.NewHashReference("refs/heads/dev", missing)
	if err := st.CheckAndSetReference(ref, nil); err != nil {
		t.Fatal(err)
	}
	iter, err := st.IterEncodedObjects(plumbing.CommitObject)
	if err != nil {
		t.Fatal(err)
	}
	if err := iter.ForEach(func(plumbing.EncodedObject) error { return storer.ErrStop }); err != nil {
		t.Fatal(err)
	}
	mem := objecttest.NewMem()
	raw, err := Open(mem, newMemRefs(), "acct", "repo_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.Put(context.Background(), objstore.LooseObjectKey("acct", "repo_1", missing.String()), bytes.NewReader([]byte("nope")), 4); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.EncodedObject(plumbing.AnyObject, missing); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestOpenErrors(t *testing.T) {
	t.Parallel()
	if _, err := Open(nil, newMemRefs(), "a", "r"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Open(objecttest.NewMem(), nil, "a", "r"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Open(objecttest.NewMem(), newMemRefs(), "", "r"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBlobTreeCommit(t *testing.T) {
	t.Parallel()
	st := mustOpen(t)
	blobH := putBlob(t, st, "hello")
	treeH := putTree(t, st, blobH)
	commitH := putCommit(t, st, treeH)

	got, err := st.EncodedObject(plumbing.BlobObject, blobH)
	if err != nil {
		t.Fatal(err)
	}
	r, err := got.Reader()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil || string(body) != "hello" {
		t.Fatalf("%q %v", body, err)
	}
	if err := st.HasEncodedObject(commitH); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EncodedObjectSize(treeH); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EncodedObject(plumbing.BlobObject, commitH); err != plumbing.ErrObjectNotFound {
		t.Fatalf("type mismatch: %v", err)
	}
	if err := st.HasEncodedObject(plumbing.NewHash("0123456789abcdef0123456789abcdef01234567")); err != plumbing.ErrObjectNotFound {
		t.Fatalf("missing: %v", err)
	}
	iter, err := st.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	if err := iter.ForEach(func(plumbing.EncodedObject) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("iter %d", n)
	}
}

func TestRefsCAS(t *testing.T) {
	t.Parallel()
	st := mustStore(t)
	h1 := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	h2 := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err := st.SetReference(plumbing.NewHashReference("refs/heads/main", h1)); err != nil {
		t.Fatal(err)
	}
	got, err := st.Reference("refs/heads/main")
	if err != nil || got.Hash() != h1 {
		t.Fatalf("%v %v", got, err)
	}
	old := plumbing.NewHashReference("refs/heads/main", h1)
	next := plumbing.NewHashReference("refs/heads/main", h2)
	if err := st.CheckAndSetReference(next, old); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckAndSetReference(old, old); err != storage.ErrReferenceHasChanged {
		t.Fatalf("cas: %v", err)
	}
	assertSymbolicHead(t, st)
	assertRefCleanup(t, st)
}

func assertSymbolicHead(t *testing.T, st *Store) {
	t.Helper()
	if err := st.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")); err != nil {
		t.Fatal(err)
	}
	got, err := st.Reference(plumbing.HEAD)
	if err != nil || got.Target() != "refs/heads/main" {
		t.Fatalf("head %v %v", got, err)
	}
	n, err := st.CountLooseRefs()
	if err != nil || n != 2 {
		t.Fatalf("count %d %v", n, err)
	}
	iter, err := st.IterReferences()
	if err != nil {
		t.Fatal(err)
	}
	if err := iter.ForEach(func(*plumbing.Reference) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func assertRefCleanup(t *testing.T, st *Store) {
	t.Helper()
	if err := st.RemoveReference("refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Reference("refs/heads/main"); err != plumbing.ErrReferenceNotFound {
		t.Fatalf("removed: %v", err)
	}
	if err := st.PackRefs(); err != nil || st.AddAlternate("x") == nil {
		t.Fatal(err)
	}
	if err := st.SetReference(nil); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckAndSetReference(nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveReference("refs/heads/ghost"); err != nil {
		t.Fatal(err)
	}
	if shaFromLooseKey("x") != "" {
		t.Fatal("short key")
	}
}

func mustStore(t *testing.T) *Store {
	t.Helper()
	raw, err := Open(objecttest.NewMem(), newMemRefs(), "acct", "repo_1")
	if err != nil {
		t.Fatal(err)
	}
	return raw.(*Store)
}

func TestPackRoundTrip(t *testing.T) {
	t.Parallel()
	src := mustOpen(t)
	blobH := putBlob(t, src, "packed")
	treeH := putTree(t, src, blobH)
	commitH := putCommit(t, src, treeH)

	var buf bytesBuf
	enc := packfile.NewEncoder(&buf, src, false)
	if _, err := enc.Encode([]plumbing.Hash{commitH}, 10); err != nil {
		t.Fatal(err)
	}
	dst := mustOpen(t)
	w, err := dst.(*Store).PackfileWriter()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := dst.EncodedObject(plumbing.CommitObject, commitH)
	if err != nil || got.Hash() != commitH {
		t.Fatalf("%v %v", got, err)
	}
}

type bytesBuf struct{ b []byte }

func (w *bytesBuf) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
func (w *bytesBuf) Bytes() []byte { return w.b }

func mustOpen(t *testing.T) storer.Storer {
	t.Helper()
	st, err := Open(objecttest.NewMem(), newMemRefs(), "acct", "repo_1")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func putBlob(t *testing.T, st storer.Storer, body string) plumbing.Hash {
	t.Helper()
	obj := st.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	h, err := st.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func putTree(t *testing.T, st storer.Storer, blob plumbing.Hash) plumbing.Hash {
	t.Helper()
	tr := object.Tree{Entries: []object.TreeEntry{{Name: "f", Mode: filemode.Regular, Hash: blob}}}
	obj := st.NewEncodedObject()
	if err := tr.Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := st.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func putCommit(t *testing.T, st storer.Storer, tree plumbing.Hash) plumbing.Hash {
	t.Helper()
	when := time.Unix(1_700_000_000, 0).UTC()
	c := object.Commit{
		Author:    object.Signature{Name: "a", Email: "a@b", When: when},
		Committer: object.Signature{Name: "a", Email: "a@b", When: when},
		Message:   "msg\n",
		TreeHash:  tree,
	}
	obj := st.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := st.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
