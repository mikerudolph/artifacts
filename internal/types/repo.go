package types

import "time"

const DefaultBranch = "main"

// Repo is the durable repository record.
type Repo struct {
	ID            RepoID        `json:"id"`
	NamespaceID   NamespaceID   `json:"-"`
	AccountID     AccountID     `json:"-"`
	Namespace     NamespaceName `json:"-"`
	Name          RepoName      `json:"name"`
	Description   string        `json:"description"`
	DefaultBranch string        `json:"default_branch"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	LastPushAt    *time.Time    `json:"last_push_at"`
	Source        string        `json:"source"`
	ReadOnly      bool          `json:"read_only"`
	Status        RepoStatus    `json:"-"`
	Remote        string        `json:"remote"`
}

// Namespace is the durable namespace record.
type Namespace struct {
	ID           NamespaceID   `json:"-"`
	AccountID    AccountID     `json:"-"`
	Name         NamespaceName `json:"namespace"`
	Jurisdiction Jurisdiction  `json:"jurisdiction,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// CreateRepoInput is the control-plane create body.
type CreateRepoInput struct {
	Name          RepoName `json:"name"`
	Description   string   `json:"description"`
	DefaultBranch string   `json:"default_branch"`
	ReadOnly      bool     `json:"read_only"`
}

// ForkRepoInput is the control-plane fork body.
type ForkRepoInput struct {
	Name              RepoName `json:"name"`
	Description       string   `json:"description"`
	ReadOnly          bool     `json:"read_only"`
	DefaultBranchOnly bool     `json:"default_branch_only"`
}

// ImportRepoInput is the control-plane import body.
type ImportRepoInput struct {
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	Depth    int    `json:"depth"`
	ReadOnly bool   `json:"read_only"`
}

// CreateRepoResult is returned by create, fork, and import.
type CreateRepoResult struct {
	ID            RepoID   `json:"id"`
	Name          RepoName `json:"name"`
	Description   *string  `json:"description"`
	DefaultBranch string   `json:"default_branch"`
	Remote        string   `json:"remote"`
	Token         string   `json:"token"`
	Objects       int      `json:"objects,omitempty"`
}
