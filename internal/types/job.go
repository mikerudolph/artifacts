package types

import "time"

type JobKind string

const (
	JobImport JobKind = "import"
	JobFork   JobKind = "fork"
	JobDelete JobKind = "delete"
)

type JobPhase string

const (
	JobQueued    JobPhase = "queued"
	JobRunning   JobPhase = "running"
	JobSucceeded JobPhase = "succeeded"
	JobFailed    JobPhase = "failed"
)

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
