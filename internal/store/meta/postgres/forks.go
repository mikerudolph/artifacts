package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s forkStore) CreateSnapshot(ctx context.Context, source types.RepoID, dest types.Repo, defaultOnly bool) (types.Repo, error) {
	if s.p != nil {
		var created types.Repo
		err := s.RunInTx(ctx, func(tx meta.Store) error {
			v2, ok := tx.(meta.V2Store)
			if !ok {
				return errors.New("v2 transaction unavailable")
			}
			var err error
			created, err = v2.Forks().CreateSnapshot(ctx, source, dest, defaultOnly)
			return err
		})
		return created, err
	}
	var account types.AccountID
	var sequence int64
	var branch string
	var status types.RepoStatus
	err := s.q.QueryRow(ctx, `SELECT account_id,wal_sequence,default_branch,status FROM repos WHERE id=$1 FOR SHARE`, source).
		Scan(&account, &sequence, &branch, &status)
	if err != nil {
		return types.Repo{}, wrap(err)
	}
	if status != types.RepoReady || (dest.AccountID != "" && dest.AccountID != account) {
		return types.Repo{}, meta.ErrCASConflict
	}
	dest.AccountID = account
	dest.Status = types.RepoReady
	created, err := s.Repos().Create(ctx, dest)
	if err != nil {
		return types.Repo{}, err
	}
	_, err = s.q.Exec(ctx, `INSERT INTO repo_forks (repo_id,parent_repo_id,parent_sequence,created_at) VALUES ($1,$2,$3,$4)`,
		created.ID, source, sequence, time.Now().UTC())
	if err != nil {
		return types.Repo{}, err
	}
	filter := ""
	args := []any{created.ID, source}
	if defaultOnly {
		filter = ` AND (name='HEAD' OR name=$3)`
		args = append(args, "refs/heads/"+branch)
	}
	_, err = s.q.Exec(ctx, `INSERT INTO refs (repo_id,name,sha) SELECT $1,name,sha FROM refs WHERE repo_id=$2`+filter, args...)
	return created, err
}

func (s forkStore) Get(ctx context.Context, repo types.RepoID) (types.ForkLineage, error) {
	var line types.ForkLineage
	err := s.q.QueryRow(ctx, `SELECT repo_id,parent_repo_id,parent_sequence,created_at FROM repo_forks WHERE repo_id=$1`, repo).
		Scan(&line.RepoID, &line.ParentRepoID, &line.ParentSequence, &line.CreatedAt)
	return line, wrap(err)
}

func (s forkStore) Children(ctx context.Context, repo types.RepoID) ([]types.ForkLineage, error) {
	rows, err := s.q.Query(ctx, `SELECT repo_id,parent_repo_id,parent_sequence,created_at FROM repo_forks WHERE parent_repo_id=$1 ORDER BY created_at`, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.ForkLineage
	for rows.Next() {
		var line types.ForkLineage
		if err := rows.Scan(&line.RepoID, &line.ParentRepoID, &line.ParentSequence, &line.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}
