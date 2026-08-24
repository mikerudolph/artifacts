package postgres

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s refStore) Get(ctx context.Context, repoID types.RepoID, name string) (types.Ref, error) {
	var ref types.Ref
	err := s.q.QueryRow(ctx, `SELECT repo_id, name, sha FROM refs WHERE repo_id = $1 AND name = $2`,
		string(repoID), name).Scan(&ref.RepoID, &ref.Name, &ref.SHA)
	if err != nil {
		return types.Ref{}, wrap(err)
	}
	return ref, nil
}

func (s refStore) List(ctx context.Context, repoID types.RepoID) ([]types.Ref, error) {
	rows, err := s.q.Query(ctx, `SELECT repo_id, name, sha FROM refs WHERE repo_id = $1 ORDER BY name`, string(repoID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Ref
	for rows.Next() {
		var ref types.Ref
		if err := rows.Scan(&ref.RepoID, &ref.Name, &ref.SHA); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (s refStore) CompareAndSwap(ctx context.Context, repoID types.RepoID, name, oldSHA, newSHA string) error {
	switch {
	case oldSHA == "" && newSHA == "":
		return nil
	case oldSHA == "":
		return s.casCreate(ctx, repoID, name, newSHA)
	case newSHA == "":
		return s.casDelete(ctx, repoID, name, oldSHA)
	default:
		return s.casUpdate(ctx, repoID, name, oldSHA, newSHA)
	}
}

func (s refStore) casCreate(ctx context.Context, repoID types.RepoID, name, sha string) error {
	tag, err := s.q.Exec(ctx, `INSERT INTO refs (repo_id, name, sha) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		string(repoID), name, sha)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return meta.ErrCASConflict
	}
	return nil
}

func (s refStore) casUpdate(ctx context.Context, repoID types.RepoID, name, oldSHA, newSHA string) error {
	tag, err := s.q.Exec(ctx, `UPDATE refs SET sha = $4 WHERE repo_id = $1 AND name = $2 AND sha = $3`,
		string(repoID), name, oldSHA, newSHA)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return meta.ErrCASConflict
	}
	return nil
}

func (s refStore) casDelete(ctx context.Context, repoID types.RepoID, name, oldSHA string) error {
	tag, err := s.q.Exec(ctx, `DELETE FROM refs WHERE repo_id = $1 AND name = $2 AND sha = $3`,
		string(repoID), name, oldSHA)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return meta.ErrCASConflict
	}
	return nil
}

func (s refStore) DeleteAll(ctx context.Context, repoID types.RepoID) error {
	_, err := s.q.Exec(ctx, `DELETE FROM refs WHERE repo_id = $1`, string(repoID))
	return err
}
