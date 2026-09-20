package jobs

import (
	"context"
	"errors"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (r *Runner) Fork(ctx context.Context, account types.AccountID, srcNS, srcName string, dst types.ForkRepoInput) (types.CreateRepoResult, error) {
	src, nspace, err := r.lookup(ctx, account, srcNS, srcName)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	if err := busy(src.Status); err != nil {
		return types.CreateRepoResult{}, err
	}
	if src.StorageVersion == 1 {
		if r.upgrades == nil {
			return types.CreateRepoResult{}, errors.New("legacy repository upgrade unavailable")
		}
		src, err = r.upgrades.Upgrade(ctx, src)
		if err != nil {
			return types.CreateRepoResult{}, err
		}
	}
	name, err := types.ParseRepoName(string(dst.Name))
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	destInput := types.Repo{
		NamespaceID: nspace.ID, AccountID: account, Name: name, Description: dst.Description,
		DefaultBranch: src.DefaultBranch, ReadOnly: dst.ReadOnly, Status: types.RepoReady, CreatedAt: r.now(),
	}
	dest, err := r.createSnapshot(ctx, src, destInput, dst.DefaultBranchOnly)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	job, err := r.startJob(ctx, dest.ID, types.JobFork)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	tok, err := r.mint(ctx, dest.ID)
	if err != nil {
		return types.CreateRepoResult{}, r.finishJob(ctx, job, err)
	}
	if err := r.finishJob(ctx, job, nil); err != nil {
		return types.CreateRepoResult{}, err
	}
	return types.CreateRepoResult{
		ID: dest.ID, Name: dest.Name, Description: descPtr(dest.Description),
		DefaultBranch: dest.DefaultBranch, Remote: r.tenantRemote(account, nspace.Name, dest.Name),
		Token: tok.Plaintext, Credential: &tok,
	}, nil
}

func (r *Runner) createSnapshot(ctx context.Context, source, dest types.Repo, defaultOnly bool) (types.Repo, error) {
	if v2, ok := r.meta.(meta.V2Store); ok {
		return v2.Forks().CreateSnapshot(ctx, source.ID, dest, defaultOnly)
	}
	var created types.Repo
	err := r.meta.RunInTx(ctx, func(tx meta.Store) error {
		var err error
		created, err = tx.Repos().Create(ctx, dest)
		if err != nil {
			return err
		}
		refs, err := tx.Refs().List(ctx, source.ID)
		if err != nil {
			return err
		}
		want := "refs/heads/" + source.DefaultBranch
		for _, ref := range refs {
			if defaultOnly && ref.Name != "HEAD" && ref.Name != want {
				continue
			}
			if err := tx.Refs().CompareAndSwap(ctx, created.ID, ref.Name, "", ref.SHA); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

func descPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
