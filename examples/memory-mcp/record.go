package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type metadata struct {
	ID           string   `json:"id" yaml:"id"`
	Type         string   `json:"type" yaml:"type"`
	Kind         string   `json:"kind,omitempty" yaml:"kind,omitempty"`
	Operation    string   `json:"operation,omitempty" yaml:"operation,omitempty"`
	RecordedAt   string   `json:"recorded_at" yaml:"recorded_at"`
	Title        string   `json:"title,omitempty" yaml:"title,omitempty"`
	Tags         []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	ThreadIDs    []string `json:"thread_ids,omitempty" yaml:"threads,omitempty"`
	SourceRunID  string   `json:"source_run_id,omitempty" yaml:"source_run,omitempty"`
	Supersedes   string   `json:"supersedes,omitempty" yaml:"supersedes,omitempty"`
	Retracts     string   `json:"retracts,omitempty" yaml:"retracts,omitempty"`
	Reason       string   `json:"reason,omitempty" yaml:"reason,omitempty"`
	Agent        string   `json:"agent,omitempty" yaml:"agent,omitempty"`
	Status       string   `json:"status,omitempty" yaml:"status,omitempty"`
	ParentRunID  string   `json:"parent_run_id,omitempty" yaml:"parent_run,omitempty"`
	FinishedAt   string   `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	RunID        string   `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	Name         string   `json:"name,omitempty" yaml:"name,omitempty"`
	MediaType    string   `json:"media_type,omitempty" yaml:"media_type,omitempty"`
	SHA256       string   `json:"sha256,omitempty" yaml:"sha256,omitempty"`
	Size         int      `json:"size,omitempty" yaml:"size,omitempty"`
	StoredEncode string   `json:"stored_encoding,omitempty" yaml:"stored_encoding,omitempty"`
	temporal     `yaml:",inline"`
}

type document struct {
	Metadata metadata `json:"metadata"`
	Content  string   `json:"content"`
	Path     string   `json:"-"`
}

func newID(prefix string, now time.Time, source io.Reader) (string, error) {
	data := make([]byte, 16)
	nanos := uint64(now.UnixNano()) //nolint:gosec // ordering, not a secret
	for i := 7; i >= 0; i-- {
		data[i] = byte(nanos)
		nanos >>= 8
	}
	if _, err := io.ReadFull(source, data[8:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data)
	return prefix + strings.ToLower(encoded), nil
}

func generateID(prefix string, now time.Time) (string, error) {
	return newID(prefix, now, rand.Reader)
}

func renderDocument(doc document) ([]byte, error) {
	front, err := yaml.Marshal(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encode front matter: %w", err)
	}
	out := make([]byte, 0, len(front)+len(doc.Content)+10)
	out = append(out, []byte("---\n")...)
	out = append(out, front...)
	out = append(out, []byte("---\n")...)
	out = append(out, doc.Content...)
	return out, nil
}

func parseDocument(data []byte) (document, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return document{}, errors.New("missing YAML front matter")
	}
	end := bytes.Index(data[4:], []byte("---\n"))
	if end < 0 {
		return document{}, errors.New("unterminated YAML front matter")
	}
	end += 4
	var meta metadata
	if err := yaml.Unmarshal(data[4:end], &meta); err != nil {
		return document{}, errors.New("invalid YAML front matter")
	}
	if meta.ID == "" || meta.Type == "" || meta.RecordedAt == "" {
		return document{}, errors.New("incomplete front matter")
	}
	return document{Metadata: meta, Content: string(data[end+4:])}, nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
