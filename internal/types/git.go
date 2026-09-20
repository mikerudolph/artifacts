package types

import "time"

type Signature struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	When  time.Time `json:"date"`
}

type Commit struct {
	Hash      string    `json:"hash"`
	Tree      string    `json:"tree"`
	Parents   []string  `json:"parents"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
	Message   string    `json:"message"`
}

type TreeEntry struct {
	Mode string `json:"mode"`
	Type string `json:"type"`
	Hash string `json:"hash"`
	Name string `json:"name"`
}

type LogEntry struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
}

type Ref struct {
	RepoID RepoID `json:"-"`
	Name   string `json:"name"`
	SHA    string `json:"sha"`
}
