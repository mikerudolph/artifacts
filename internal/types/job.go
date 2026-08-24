package types

import "time"

// JobKind is a background control-plane job.
type JobKind string

const (
	JobImport JobKind = "import"
	JobFork   JobKind = "fork"
	JobDelete JobKind = "delete"
)

// JobPhase is a background job lifecycle.
type JobPhase string

const (
	JobQueued    JobPhase = "queued"
	JobRunning   JobPhase = "running"
	JobSucceeded JobPhase = "succeeded"
	JobFailed    JobPhase = "failed"
)

// Job is a durable background job.
type Job struct {
	ID        JobID     `json:"id"`
	RepoID    RepoID    `json:"repo_id"`
	Kind      JobKind   `json:"kind"`
	Status    JobPhase  `json:"status"`
	Error     string    `json:"error,omitempty"`
	Progress  int       `json:"progress"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
