package types

import "time"

// ImportSpec constrains one remote import to a validated, pinned HTTPS origin.
type ImportSpec struct {
	URL           string
	Branch        string
	PinnedAddress string
	Depth         int
	MaxBytes      int64
	MaxObjects    int
	Timeout       time.Duration
}

// CommitFile is one bounded REST commit file.
type CommitFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CommitInput publishes files through the same pack WAL as Git receive.
type CommitInput struct {
	Branch  string       `json:"branch"`
	Message string       `json:"message"`
	Author  Signature    `json:"author"`
	Files   []CommitFile `json:"files"`
}

// CommitResult identifies a REST-published commit.
type CommitResult struct {
	SHA      string `json:"sha"`
	Sequence int64  `json:"sequence"`
}

// RefUpdate is one expected atomic reference transition.
type RefUpdate struct {
	Name   string `json:"name"`
	OldSHA string `json:"old_sha"`
	NewSHA string `json:"new_sha"`
}

// PackWAL is an immutable published Git pack and index pair.
type PackWAL struct {
	RepoID    RepoID    `json:"-"`
	Sequence  int64     `json:"sequence"`
	PackKey   string    `json:"pack_key"`
	IndexKey  string    `json:"index_key"`
	Checksum  string    `json:"checksum"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Publication atomically records a durable pack and its ref transitions.
type Publication struct {
	Pack             PackWAL     `json:"pack"`
	Updates          []RefUpdate `json:"updates"`
	ExpectedSequence int64       `json:"expected_sequence"`
	AllowReadOnly    bool        `json:"-"`
	AllowNonReady    bool        `json:"-"`
	UpgradeFrom      int         `json:"-"`
}

// Checkpoint identifies a compacted immutable repository pack.
type Checkpoint struct {
	RepoID    RepoID    `json:"-"`
	Sequence  int64     `json:"sequence"`
	PackKey   string    `json:"pack_key"`
	IndexKey  string    `json:"index_key"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
}

// ForkLineage fixes a snapshot fork to a parent publication sequence.
type ForkLineage struct {
	RepoID         RepoID    `json:"-"`
	ParentRepoID   RepoID    `json:"parent_repo_id"`
	ParentSequence int64     `json:"parent_sequence"`
	CreatedAt      time.Time `json:"created_at"`
}
