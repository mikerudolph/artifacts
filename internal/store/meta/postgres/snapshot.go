package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *store) RepositorySnapshot(ctx context.Context, id types.RepoID) (types.Repo, []types.Ref, error) {
	query := strings.Replace(repoSelect, " FROM repos", `, COALESCE(
 (SELECT jsonb_agg(jsonb_build_object('name', name, 'sha', sha) ORDER BY name)
 FROM refs WHERE repo_id = repos.id), '[]'::jsonb) FROM repos`, 1)
	var data []byte
	repo, err := scanRepo(s.q.QueryRow(ctx, query+" WHERE id=$1", id), &data)
	if err != nil {
		return types.Repo{}, nil, wrap(err)
	}
	var refs []types.Ref
	if err := json.Unmarshal(data, &refs); err != nil {
		return types.Repo{}, nil, err
	}
	for i := range refs {
		refs[i].RepoID = id
	}
	return repo, refs, nil
}
