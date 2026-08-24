package jobs

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Fork copies objects and refs into a new repo.
func (r *Runner) Fork(ctx context.Context, account types.AccountID, srcNS, srcName string, dst types.ForkRepoInput) (types.CreateRepoResult, error) {
	src, nspace, err := r.lookup(ctx, account, srcNS, srcName)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	if err := busy(src.Status); err != nil {
		return types.CreateRepoResult{}, err
	}
	name, err := types.ParseRepoName(string(dst.Name))
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	dest, err := r.meta.Repos().Create(ctx, types.Repo{
		NamespaceID: nspace.ID, AccountID: account, Name: name, Description: dst.Description,
		DefaultBranch: src.DefaultBranch, ReadOnly: dst.ReadOnly, Status: types.RepoForking, CreatedAt: r.now(),
	})
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	n, err := r.copyRepo(ctx, account, src, dest, dst.DefaultBranchOnly)
	if err != nil {
		dest.Status = types.RepoReady
		_, _ = r.meta.Repos().Update(ctx, dest)
		return types.CreateRepoResult{}, err
	}
	dest.Status = types.RepoReady
	if _, err := r.meta.Repos().Update(ctx, dest); err != nil {
		return types.CreateRepoResult{}, err
	}
	tok, err := r.mint(ctx, dest.ID)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	return types.CreateRepoResult{
		ID: dest.ID, Name: dest.Name, Description: descPtr(dest.Description),
		DefaultBranch: dest.DefaultBranch, Remote: r.remote(nspace.Name, dest.Name),
		Token: tok.Plaintext, Objects: n,
	}, nil
}

func (r *Runner) copyRepo(ctx context.Context, account types.AccountID, src, dest types.Repo, defaultOnly bool) (int, error) {
	prefix := object.ObjectsPrefix(string(account), string(src.ID)) + "/"
	keys, err := r.objects.List(ctx, prefix)
	if err != nil {
		return 0, err
	}
	dstPref := object.ObjectsPrefix(string(account), string(dest.ID))
	srcPref := object.ObjectsPrefix(string(account), string(src.ID))
	for _, key := range keys {
		dst := dstPref + key[len(srcPref):]
		if err := r.objects.Copy(ctx, key, dst); err != nil {
			return 0, err
		}
	}
	refs, err := r.meta.Refs().List(ctx, src.ID)
	if err != nil {
		return 0, err
	}
	want := "refs/heads/" + src.DefaultBranch
	for _, ref := range refs {
		if defaultOnly && ref.Name != want && ref.Name != "HEAD" {
			continue
		}
		if err := r.meta.Refs().CompareAndSwap(ctx, dest.ID, ref.Name, "", ref.SHA); err != nil {
			return 0, err
		}
	}
	return len(keys), nil
}

func descPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
