package jobs

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Delete removes objects, refs, and the repo row.
func (r *Runner) Delete(ctx context.Context, account types.AccountID, ns, name string) error {
	repo, _, err := r.lookup(ctx, account, ns, name)
	if err != nil {
		return err
	}
	repo.Status = types.RepoDeleting
	if _, err := r.meta.Repos().Update(ctx, repo); err != nil {
		return err
	}
	if err := r.objects.DeletePrefix(ctx, object.RepoPrefix(string(account), string(repo.ID))); err != nil {
		return err
	}
	if err := r.meta.Refs().DeleteAll(ctx, repo.ID); err != nil {
		return err
	}
	return r.meta.Repos().Delete(ctx, repo.ID)
}
