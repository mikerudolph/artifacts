package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestImportErrorsAndMap(t *testing.T) {
	r, _ := testRunner(t)
	ctx := context.Background()
	if _, err := r.Import(ctx, "local", "-bad", "x", types.ImportRepoInput{URL: "http://example"}); err == nil {
		t.Fatal("ns")
	}
	if _, err := r.Import(ctx, "local", "default", "-x", types.ImportRepoInput{URL: "http://example"}); err == nil {
		t.Fatal("name")
	}
	if _, err := r.Import(ctx, "local", "default", "gone", types.ImportRepoInput{URL: "http://127.0.0.1:1/no.git"}); err != ErrUpstream {
		t.Fatalf("upstream %v", err)
	}
	if _, err := r.Fork(ctx, "local", "default", "missing", types.ForkRepoInput{Name: "d"}); err == nil {
		t.Fatal("missing src")
	}
	if mapCloneErr(nil) != nil {
		t.Fatal("nil")
	}
	if mapCloneErr(transport.ErrAuthenticationRequired) != ErrRemoteAuth {
		t.Fatal("auth")
	}
	if mapCloneErr(errString("repository not found")) != ErrInvalidURL {
		t.Fatal("not found")
	}
	if mapCloneErr(errString("invalid remote")) != ErrInvalidURL {
		t.Fatal("invalid")
	}
	if firstNonEmpty("", "d") != "d" || firstNonEmpty("a", "d") != "a" {
		t.Fatal("first")
	}
	if _, _, err := r.lookup(ctx, "local", "-n", "x"); err == nil {
		t.Fatal("lookup ns")
	}
	if _, _, err := r.lookup(ctx, "local", "default", "-r"); err == nil {
		t.Fatal("lookup repo")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

type failEnc struct{}

func (failEnc) NewEncodedObject() plumbing.EncodedObject { return nil }
func (failEnc) SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error) {
	return plumbing.ZeroHash, nil
}
func (failEnc) EncodedObject(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error) {
	return nil, nil
}
func (failEnc) IterEncodedObjects(plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	return nil, errors.New("iter")
}
func (failEnc) HasEncodedObject(plumbing.Hash) error           { return nil }
func (failEnc) EncodedObjectSize(plumbing.Hash) (int64, error) { return 0, nil }
func (failEnc) AddAlternate(string) error                      { return nil }

func TestCopyStorerIterErr(t *testing.T) {
	t.Parallel()
	if err := copyStorer(failEnc{}, memory.NewStorage()); err == nil {
		t.Fatal("expected iter error")
	}
}

func TestCopyStorerEmpty(t *testing.T) {
	t.Parallel()
	src := memory.NewStorage()
	dst := memory.NewStorage()
	if err := copyStorer(src, dst); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteExisting(t *testing.T) {
	r, _ := testRunner(t)
	ctx := context.Background()
	_ = r.meta.Accounts().Ensure(ctx, "local")
	ns, err := r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: "local", Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.meta.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "gone", DefaultBranch: "main", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, "local", "default", "gone"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMissing(t *testing.T) {
	r, _ := testRunner(t)
	if err := r.Delete(context.Background(), "local", "default", "nope"); err == nil {
		t.Fatal("expected missing")
	}
}

func TestForkBadName(t *testing.T) {
	r, _ := testRunner(t)
	ctx := context.Background()
	_ = r.meta.Accounts().Ensure(ctx, "local")
	ns, err := r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: "local", Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.meta.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "src", DefaultBranch: "main", Status: types.RepoReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "-bad"}); err == nil {
		t.Fatal("expected name")
	}
	src, _ := r.meta.Repos().GetByName(ctx, ns.ID, "src")
	src.Status = types.RepoForking
	_, _ = r.meta.Repos().Update(ctx, src)
	if _, err := r.Fork(ctx, "local", "default", "src", types.ForkRepoInput{Name: "dst"}); err != ErrBusy {
		t.Fatalf("busy %v", err)
	}
	if descPtr("") != nil || descPtr("x") == nil {
		t.Fatal("descPtr")
	}
}
