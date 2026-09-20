package types

import "time"

type PublicationEvent struct {
	RepoID    RepoID      `json:"repo_id"`
	Sequence  int64       `json:"sequence"`
	CreatedAt time.Time   `json:"created_at"`
	Updates   []RefUpdate `json:"updates"`
}

type EventPage struct {
	Events    []PublicationEvent `json:"events"`
	NextAfter int64              `json:"next_after"`
}
