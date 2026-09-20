package postgres

import (
	"context"
	"encoding/json"

	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *store) PublicationEvents(ctx context.Context, repo types.RepoID, after int64, limit int) ([]types.PublicationEvent, error) {
	rows, err := s.q.Query(ctx, `WITH page AS (
 SELECT repo_id, sequence, created_at FROM pack_wal WHERE repo_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3
 ) SELECT p.repo_id,p.sequence,p.created_at,COALESCE((SELECT jsonb_agg(jsonb_build_object(
 'name',u.name,'old_sha',u.old_sha,'new_sha',u.new_sha) ORDER BY u.name)
 FROM pack_ref_updates u WHERE u.repo_id=p.repo_id AND u.sequence=p.sequence),'[]'::jsonb)
 FROM page p ORDER BY p.sequence`, repo, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []types.PublicationEvent{}
	for rows.Next() {
		var event types.PublicationEvent
		var updates []byte
		if err := rows.Scan(&event.RepoID, &event.Sequence, &event.CreatedAt, &updates); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(updates, &event.Updates); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *store) CompactionCandidates(ctx context.Context, threshold int64, limit int) ([]types.RepoID, error) {
	rows, err := s.q.Query(ctx, `SELECT r.id FROM repos r LEFT JOIN checkpoints c ON c.repo_id=r.id
 WHERE r.status='ready' AND r.storage_version=2 AND r.wal_sequence-COALESCE(c.sequence,0)>=$1
 ORDER BY r.updated_at,r.id LIMIT $2`, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.RepoID
	for rows.Next() {
		var id types.RepoID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
