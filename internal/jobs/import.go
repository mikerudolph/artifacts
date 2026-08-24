package jobs

import (
	"context"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Import clones a public HTTPS (or file) remote into a new repo.
func (r *Runner) Import(ctx context.Context, account types.AccountID, ns string, name types.RepoName, in types.ImportRepoInput) (types.CreateRepoResult, error) {
	if in.URL == "" || strings.Contains(in.URL, " ") {
		return types.CreateRepoResult{}, ErrInvalidURL
	}
	nsName, err := types.ParseNamespaceName(ns)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	repoName, err := types.ParseRepoName(string(name))
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	if err := r.meta.Accounts().Ensure(ctx, account); err != nil {
		return types.CreateRepoResult{}, err
	}
	nspace, err := r.ensureNS(ctx, account, nsName)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	repo, err := r.meta.Repos().Create(ctx, types.Repo{
		NamespaceID: nspace.ID, AccountID: account, Name: repoName, Source: in.URL,
		DefaultBranch: firstNonEmpty(in.Branch, types.DefaultBranch), ReadOnly: in.ReadOnly,
		Status: types.RepoImporting, CreatedAt: r.now(),
	})
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	if err := r.cloneInto(ctx, account, repo, in); err != nil {
		repo.Status = types.RepoReady
		_, _ = r.meta.Repos().Update(ctx, repo)
		return types.CreateRepoResult{}, err
	}
	repo.Status = types.RepoReady
	if _, err := r.meta.Repos().Update(ctx, repo); err != nil {
		return types.CreateRepoResult{}, err
	}
	tok, err := r.mint(ctx, repo.ID)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	return types.CreateRepoResult{
		ID: repo.ID, Name: repo.Name, DefaultBranch: repo.DefaultBranch,
		Remote: r.remote(nsName, repo.Name), Token: tok.Plaintext,
	}, nil
}

func (r *Runner) cloneInto(ctx context.Context, account types.AccountID, repo types.Repo, in types.ImportRepoInput) error {
	mem := memory.NewStorage()
	opts := &gogit.CloneOptions{URL: in.URL, Depth: in.Depth}
	if in.Branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(in.Branch)
		opts.SingleBranch = true
	}
	if _, err := gogit.CloneContext(ctx, mem, nil, opts); err != nil {
		return mapCloneErr(err)
	}
	dst, err := gitstore.Open(r.objects, r.meta.Refs(), account, repo.ID)
	if err != nil {
		return err
	}
	return copyStorer(mem, dst)
}

func copyStorer(src storer.EncodedObjectStorer, dst storer.Storer) error {
	iter, err := src.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return err
	}
	defer iter.Close()
	if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
		_, err := dst.SetEncodedObject(obj)
		return err
	}); err != nil {
		return err
	}
	ri, err := src.(storer.ReferenceStorer).IterReferences()
	if err != nil {
		return err
	}
	defer ri.Close()
	return ri.ForEach(func(ref *plumbing.Reference) error {
		return dst.SetReference(ref)
	})
}

func mapCloneErr(err error) error {
	if err == nil {
		return nil
	}
	if err == transport.ErrAuthenticationRequired {
		return ErrRemoteAuth
	}
	msg := err.Error()
	if strings.Contains(msg, "repository not found") || strings.Contains(strings.ToLower(msg), "invalid") {
		return ErrInvalidURL
	}
	return ErrUpstream
}

func (r *Runner) ensureNS(ctx context.Context, account types.AccountID, name types.NamespaceName) (types.Namespace, error) {
	ns, err := r.meta.Namespaces().GetByName(ctx, account, name)
	if err == nil {
		return ns, nil
	}
	return r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: account, Name: name, CreatedAt: r.now()})
}

func firstNonEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
