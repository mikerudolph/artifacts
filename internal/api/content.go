package api

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) readRepo(ctx context.Context, account, ns, name string, visit func(storer.Storer) error) error {
	if s.deps.ReadGit == nil {
		return context.Canceled
	}
	if _, err := s.svc.GetRepo(ctx, types.AccountID(account), ns, name); err != nil {
		return err
	}
	return s.deps.ReadGit(ctx, account, ns, name, visit)
}

func resolveRef(st storer.Storer, ref string) (plumbing.Hash, error) {
	if ref == "" {
		ref = "HEAD"
	}
	if h := plumbing.NewHash(ref); !h.IsZero() && len(ref) == 40 {
		if err := st.HasEncodedObject(h); err == nil {
			return h, nil
		}
	}
	for _, name := range []plumbing.ReferenceName{
		plumbing.ReferenceName(ref),
		plumbing.NewBranchReferenceName(ref),
		plumbing.NewTagReferenceName(ref),
	} {
		r, err := st.Reference(name)
		if err != nil {
			continue
		}
		if r.Type() == plumbing.SymbolicReference {
			r, err = storer.ResolveReference(st, r.Name())
			if err != nil {
				return plumbing.ZeroHash, meta.ErrNotFound
			}
		}
		return r.Hash(), nil
	}
	return plumbing.ZeroHash, meta.ErrNotFound
}

func queryInt(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func fileAt(st storer.Storer, ref, path string) (*object.Blob, error) {
	h, err := resolveRef(st, ref)
	if err != nil {
		return nil, err
	}
	c, err := object.GetCommit(st, h)
	if err != nil {
		return nil, meta.ErrNotFound
	}
	tree, err := c.Tree()
	if err != nil {
		return nil, meta.ErrNotFound
	}
	ent, err := tree.FindEntry(strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, meta.ErrNotFound
	}
	blob, err := object.GetBlob(st, ent.Hash)
	if err != nil {
		return nil, meta.ErrNotFound
	}
	return blob, nil
}
