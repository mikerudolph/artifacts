package types

import "time"

// Signature is a git author or committer.
type Signature struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	When  time.Time `json:"date"`
}

// Commit is a parsed git commit object.
type Commit struct {
	Hash      string    `json:"hash"`
	Tree      string    `json:"tree"`
	Parents   []string  `json:"parents"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
	Message   string    `json:"message"`
}

// TreeEntry is one entry in a git tree.
type TreeEntry struct {
	Mode string `json:"mode"`
	Type string `json:"type"`
	Hash string `json:"hash"`
	Name string `json:"name"`
}

// LogEntry is one commit in a history listing.
type LogEntry struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
}

// Ref is a named pointer at a commit SHA.
type Ref struct {
	RepoID RepoID `json:"-"`
	Name   string `json:"name"`
	SHA    string `json:"sha"`
}
