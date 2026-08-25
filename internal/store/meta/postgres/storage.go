package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s walStore) Publish(ctx context.Context, p types.Publication) (int64, error) {
	if s.p != nil {
		var sequence int64
		err := s.RunInTx(ctx, func(tx meta.Store) error {
			v2, ok := tx.(meta.V2Store)
			if !ok {
				return errors.New("v2 transaction unavailable")
			}
			var err error
			sequence, err = v2.WAL().Publish(ctx, p)
			return err
		})
		return sequence, err
	}
	return s.publishTx(ctx, p)
}

func (s walStore) publishTx(ctx context.Context, p types.Publication) (int64, error) {
	var sequence int64
	var status types.RepoStatus
	var readOnly bool
	var storageVersion int
	err := s.q.QueryRow(ctx, `SELECT wal_sequence, status, read_only, storage_version FROM repos WHERE id=$1 FOR UPDATE`, p.Pack.RepoID).
		Scan(&sequence, &status, &readOnly, &storageVersion)
	if err != nil {
		return 0, wrap(err)
	}
	if err := s.validatePublication(ctx, p, sequence, status, readOnly, storageVersion); err != nil {
		return 0, err
	}
	sequence++
	p.Pack.Sequence = sequence
	if p.Pack.CreatedAt.IsZero() {
		p.Pack.CreatedAt = time.Now().UTC()
	}
	_, err = s.q.Exec(ctx, `INSERT INTO pack_wal
		(repo_id,sequence,pack_key,index_key,checksum,size,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		p.Pack.RepoID, sequence, p.Pack.PackKey, p.Pack.IndexKey, p.Pack.Checksum, p.Pack.Size, p.Pack.CreatedAt)
	if err != nil {
		return 0, wrap(err)
	}
	for _, update := range p.Updates {
		if err := s.applyRef(ctx, p.Pack.RepoID, sequence, update); err != nil {
			return 0, err
		}
	}
	nextVersion := storageVersion
	if p.UpgradeFrom != 0 {
		nextVersion = 2
	}
	_, err = s.q.Exec(ctx, `UPDATE repos SET wal_sequence=$2,last_push_at=$3,updated_at=$3,storage_version=$4 WHERE id=$1`,
		p.Pack.RepoID, sequence, p.Pack.CreatedAt, nextVersion)
	return sequence, err
}

func (s walStore) validatePublication(ctx context.Context, p types.Publication, sequence int64, status types.RepoStatus, readOnly bool, version int) error {
	if sequence != p.ExpectedSequence {
		return meta.ErrCASConflict
	}
	if (status != types.RepoReady && !p.AllowNonReady) || (readOnly && !p.AllowReadOnly) {
		return meta.ErrCASConflict
	}
	if p.UpgradeFrom != 0 && version != p.UpgradeFrom {
		return meta.ErrCASConflict
	}
	for _, update := range p.Updates {
		if err := s.checkRef(ctx, p.Pack.RepoID, update); err != nil {
			return err
		}
	}
	return nil
}

func (s walStore) checkRef(ctx context.Context, repo types.RepoID, update types.RefUpdate) error {
	var current string
	err := s.q.QueryRow(ctx, `SELECT sha FROM refs WHERE repo_id=$1 AND name=$2`, repo, update.Name).Scan(&current)
	if meta.IsNotFound(wrap(err)) && update.OldSHA == "" {
		return nil
	}
	if err != nil {
		return wrap(err)
	}
	if current != update.OldSHA {
		return meta.ErrCASConflict
	}
	return nil
}

func (s walStore) applyRef(ctx context.Context, repo types.RepoID, sequence int64, update types.RefUpdate) error {
	_, err := s.q.Exec(ctx, `INSERT INTO pack_ref_updates (repo_id,sequence,name,old_sha,new_sha) VALUES ($1,$2,$3,$4,$5)`,
		repo, sequence, update.Name, update.OldSHA, update.NewSHA)
	if err != nil {
		return err
	}
	if update.NewSHA == "" {
		_, err = s.q.Exec(ctx, `DELETE FROM refs WHERE repo_id=$1 AND name=$2`, repo, update.Name)
		return err
	}
	_, err = s.q.Exec(ctx, `INSERT INTO refs (repo_id,name,sha) VALUES ($1,$2,$3)
		ON CONFLICT (repo_id,name) DO UPDATE SET sha=excluded.sha`, repo, update.Name, update.NewSHA)
	return err
}

func (s walStore) List(ctx context.Context, repo types.RepoID, after, through int64) ([]types.PackWAL, error) {
	rows, err := s.q.Query(ctx, `SELECT repo_id,sequence,pack_key,index_key,checksum,size,created_at FROM pack_wal
		WHERE repo_id=$1 AND sequence>$2 AND ($3<0 OR sequence<=$3) ORDER BY sequence`, repo, after, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.PackWAL
	for rows.Next() {
		var pack types.PackWAL
		if err := rows.Scan(&pack.RepoID, &pack.Sequence, &pack.PackKey, &pack.IndexKey, &pack.Checksum, &pack.Size, &pack.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, pack)
	}
	return out, rows.Err()
}

func (s checkpointStore) Put(ctx context.Context, cp types.Checkpoint) error {
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	_, err := s.q.Exec(ctx, `INSERT INTO checkpoints (repo_id,sequence,pack_key,index_key,checksum,created_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (repo_id) DO UPDATE SET
		sequence=excluded.sequence,pack_key=excluded.pack_key,index_key=excluded.index_key,
		checksum=excluded.checksum,created_at=excluded.created_at WHERE checkpoints.sequence<=excluded.sequence`,
		cp.RepoID, cp.Sequence, cp.PackKey, cp.IndexKey, cp.Checksum, cp.CreatedAt)
	return err
}

func (s checkpointStore) Get(ctx context.Context, repo types.RepoID) (types.Checkpoint, error) {
	var cp types.Checkpoint
	err := s.q.QueryRow(ctx, `SELECT repo_id,sequence,pack_key,index_key,checksum,created_at FROM checkpoints WHERE repo_id=$1`, repo).
		Scan(&cp.RepoID, &cp.Sequence, &cp.PackKey, &cp.IndexKey, &cp.Checksum, &cp.CreatedAt)
	return cp, wrap(err)
}
