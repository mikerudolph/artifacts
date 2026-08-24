package api

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// OpenGit opens a git storer for content routes. Set by process wiring.
var OpenGit func(account, ns, repo string) (storer.Storer, error)

func init() {
	contentRoutes = func(r chi.Router) {
		r.Get("/namespaces/{namespace}/repos/{name}/log", handleLog)
		r.Get("/namespaces/{namespace}/repos/{name}/commit/{hash}", handleCommit)
		r.Get("/namespaces/{namespace}/repos/{name}/tree/{hash}", handleTree)
		r.Get("/namespaces/{namespace}/repos/{name}/blob/{hash}", handleBlob)
		r.Get("/namespaces/{namespace}/repos/{name}/file", handleFile)
		r.Get("/namespaces/{namespace}/repos/{name}/raw/{ref}/*", handleRaw)
	}
}

func openRepo(ctx context.Context, account, ns, name string) (storer.Storer, error) {
	if OpenGit == nil {
		return nil, errors.New("git store not configured")
	}
	if _, err := active.svc.GetRepo(ctx, types.AccountID(account), ns, name); err != nil {
		return nil, err
	}
	return OpenGit(account, ns, name)
}

func resolveRef(st storer.Storer, ref string) (plumbing.Hash, error) {
	if ref == "" {
		ref = types.DefaultBranch
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
