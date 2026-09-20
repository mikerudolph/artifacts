package types

import "time"

const DefaultBranch = "main"

type Repo struct {
	ID             RepoID        `json:"id"`
	NamespaceID    NamespaceID   `json:"-"`
	AccountID      AccountID     `json:"-"`
	Namespace      NamespaceName `json:"-"`
	Name           RepoName      `json:"name"`
	Description    string        `json:"description"`
	DefaultBranch  string        `json:"default_branch"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	LastPushAt     *time.Time    `json:"last_push_at"`
	Source         string        `json:"source"`
	ReadOnly       bool          `json:"read_only"`
	Status         RepoStatus    `json:"-"`
	StorageVersion int           `json:"-"`
	WALSequence    int64         `json:"wal_sequence"`
	Failure        string        `json:"failure,omitempty"`
	DeletedAt      *time.Time    `json:"deleted_at,omitempty"`
	Remote         string        `json:"remote"`
}

type Namespace struct {
	ID           NamespaceID   `json:"-"`
	AccountID    AccountID     `json:"-"`
	Name         NamespaceName `json:"namespace"`
	Jurisdiction Jurisdiction  `json:"jurisdiction,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type CreateRepoInput struct {
	IssueCredential *bool    `json:"issue_credential,omitempty"`
	IdempotencyKey  string   `json:"-"`
	Name            RepoName `json:"name"`
	Description     string   `json:"description"`
	DefaultBranch   string   `json:"default_branch"`
	ReadOnly        bool     `json:"read_only"`
}

type UpdateRepoInput struct {
	Description   *string `json:"description"`
	DefaultBranch *string `json:"default_branch"`
	ReadOnly      *bool   `json:"read_only"`
}

type ForkRepoInput struct {
	Name              RepoName `json:"name"`
	Description       string   `json:"description"`
	ReadOnly          bool     `json:"read_only"`
	DefaultBranchOnly bool     `json:"default_branch_only"`
}

type ImportRepoInput struct {
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	Depth    int    `json:"depth"`
	ReadOnly bool   `json:"read_only"`
}

type CreateRepoResult struct {
	Credential    *CreateTokenResult `json:"credential,omitempty"`
	ID            RepoID             `json:"id"`
	Name          RepoName           `json:"name"`
	Description   *string            `json:"description"`
	DefaultBranch string             `json:"default_branch"`
	Remote        string             `json:"remote"`
	Token         string             `json:"token"`
	Objects       int                `json:"objects,omitempty"`
}
