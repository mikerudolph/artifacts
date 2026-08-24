package postgres

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

func (s jobStore) Create(ctx context.Context, job types.Job) (types.Job, error) {
	if job.ID == "" {
		job.ID = types.JobID(newID())
	}
	if job.Status == "" {
		job.Status = types.JobQueued
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	job.UpdatedAt = job.CreatedAt
	err := s.q.QueryRow(ctx, `
		INSERT INTO jobs (id, repo_id, kind, status, error, progress, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, repo_id, kind, status, error, progress, created_at, updated_at`,
		string(job.ID), string(job.RepoID), string(job.Kind), string(job.Status), job.Error, job.Progress, job.CreatedAt, job.UpdatedAt,
	).Scan(&job.ID, &job.RepoID, &job.Kind, &job.Status, &job.Error, &job.Progress, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return types.Job{}, wrap(err)
	}
	return job, nil
}

func (s jobStore) Get(ctx context.Context, id types.JobID) (types.Job, error) {
	var job types.Job
	err := s.q.QueryRow(ctx, jobSelect+` WHERE id = $1`, string(id)).
		Scan(&job.ID, &job.RepoID, &job.Kind, &job.Status, &job.Error, &job.Progress, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return types.Job{}, wrap(err)
	}
	return job, nil
}

func (s jobStore) Update(ctx context.Context, job types.Job) (types.Job, error) {
	job.UpdatedAt = time.Now().UTC()
	err := s.q.QueryRow(ctx, `
		UPDATE jobs SET status=$2, error=$3, progress=$4, updated_at=$5 WHERE id=$1
		RETURNING id, repo_id, kind, status, error, progress, created_at, updated_at`,
		string(job.ID), string(job.Status), job.Error, job.Progress, job.UpdatedAt,
	).Scan(&job.ID, &job.RepoID, &job.Kind, &job.Status, &job.Error, &job.Progress, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return types.Job{}, wrap(err)
	}
	return job, nil
}

func (s jobStore) ListByRepo(ctx context.Context, repoID types.RepoID) ([]types.Job, error) {
	rows, err := s.q.Query(ctx, jobSelect+` WHERE repo_id = $1 ORDER BY created_at DESC`, string(repoID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Job
	for rows.Next() {
		var job types.Job
		if err := rows.Scan(&job.ID, &job.RepoID, &job.Kind, &job.Status, &job.Error, &job.Progress, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

const jobSelect = `SELECT id, repo_id, kind, status, error, progress, created_at, updated_at FROM jobs`
