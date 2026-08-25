package jobs

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Delete durably tombstones a repository and revokes its credentials.
func (r *Runner) Delete(ctx context.Context, account types.AccountID, ns, name string) error {
	_, err := r.DeleteRepo(ctx, account, ns, name)
	return err
}

// DeleteRepo atomically records and completes an idempotent delete job.
func (r *Runner) DeleteRepo(ctx context.Context, account types.AccountID, ns, name string) (types.RepoID, error) {
	repo, _, err := r.lookup(ctx, account, ns, name)
	if err != nil {
		return "", err
	}
	for range 3 {
		err = r.meta.RunInTx(ctx, func(tx meta.Store) error { return r.deleteTx(ctx, tx, repo.ID) })
		if !meta.IsCASConflict(err) {
			break
		}
	}
	return repo.ID, err
}

func (r *Runner) deleteTx(ctx context.Context, tx meta.Store, id types.RepoID) error {
	repo, err := tx.Repos().GetByID(ctx, id)
	if err != nil || repo.Status == types.RepoDeleted {
		return err
	}
	if repo.Status != types.RepoDeleting {
		if _, err := tx.Repos().Transition(ctx, id, repo.Status, types.RepoDeleting, nil); err != nil {
			return err
		}
	}
	job, err := deleteJob(ctx, tx, id, r.now())
	if err != nil {
		return err
	}
	page := types.OffsetPage{Page: 1, PerPage: 100}
	for {
		tokens, info, err := tx.RepoTokens().List(ctx, id, types.TokenState("all"), page)
		if err != nil {
			return err
		}
		for _, token := range tokens {
			if token.State != types.TokenRevoked {
				if err := tx.RepoTokens().Revoke(ctx, token.ID); err != nil {
					return err
				}
			}
		}
		if page.Page >= info.TotalPages {
			break
		}
		page.Page++
	}
	now := time.Now().UTC()
	if _, err := tx.Repos().Transition(ctx, id, types.RepoDeleting, types.RepoDeleted, &now); err != nil {
		return err
	}
	job.Status, job.Progress = types.JobSucceeded, 100
	_, err = tx.Jobs().Update(ctx, job)
	return err
}

func deleteJob(ctx context.Context, tx meta.Store, repo types.RepoID, now time.Time) (types.Job, error) {
	jobs, err := tx.Jobs().ListByRepo(ctx, repo)
	if err != nil {
		return types.Job{}, err
	}
	for _, job := range jobs {
		if job.Kind == types.JobDelete && job.Status != types.JobFailed {
			return job, nil
		}
	}
	return tx.Jobs().Create(ctx, types.Job{
		RepoID: repo, Kind: types.JobDelete, Status: types.JobRunning, CreatedAt: now, UpdatedAt: now,
	})
}
