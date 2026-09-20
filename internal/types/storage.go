package types

import "time"

type ImportSpec struct {
	URL           string
	Branch        string
	PinnedAddress string
	Depth         int
	MaxBytes      int64
	MaxObjects    int
	Timeout       time.Duration
}

type CommitFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type CommitInput struct {
	Deletes              []string     `json:"deletes,omitempty"`
	Replace              bool         `json:"replace,omitempty"`
	ExpectedHead         *string      `json:"expected_head,omitempty"`
	Base                 string       `json:"base,omitempty"`
	IdempotencyKey       string       `json:"-"`
	RepositoryCredential bool         `json:"-"`
	Branch               string       `json:"branch"`
	Message              string       `json:"message"`
	Author               Signature    `json:"author"`
	Files                []CommitFile `json:"files"`
}

type CommitResult struct {
	SHA      string `json:"sha"`
	Sequence int64  `json:"sequence"`
}

type RefUpdate struct {
	Name   string `json:"name"`
	OldSHA string `json:"old_sha"`
	NewSHA string `json:"new_sha"`
}

type PackWAL struct {
	RepoID    RepoID    `json:"-"`
	Sequence  int64     `json:"sequence"`
	PackKey   string    `json:"pack_key"`
	IndexKey  string    `json:"index_key"`
	Checksum  string    `json:"checksum"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type Publication struct {
	Pack             PackWAL     `json:"pack"`
	Updates          []RefUpdate `json:"updates"`
	ExpectedSequence int64       `json:"expected_sequence"`
	AllowReadOnly    bool        `json:"-"`
	AllowNonReady    bool        `json:"-"`
	UpgradeFrom      int         `json:"-"`
}

type Checkpoint struct {
	RepoID    RepoID    `json:"-"`
	Sequence  int64     `json:"sequence"`
	PackKey   string    `json:"pack_key"`
	IndexKey  string    `json:"index_key"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
}

type ForkLineage struct {
	RepoID         RepoID    `json:"-"`
	ParentRepoID   RepoID    `json:"parent_repo_id"`
	ParentSequence int64     `json:"parent_sequence"`
	CreatedAt      time.Time `json:"created_at"`
}
