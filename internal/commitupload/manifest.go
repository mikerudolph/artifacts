package commitupload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/mikerudolph/artifacts/internal/types"
)

const ManifestLimit = 256 << 10
const WireOverhead = 2 << 20

var partName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type File struct {
	Path string `json:"path"`
	Part string `json:"part"`
	Size *int64 `json:"size,omitempty"`
	Mode string `json:"mode,omitempty"`
}

type Manifest struct {
	Branch       string          `json:"branch"`
	Message      string          `json:"message"`
	Author       types.Signature `json:"author"`
	ExpectedHead *string         `json:"expected_head,omitempty"`
	Base         string          `json:"base,omitempty"`
	Replace      bool            `json:"replace,omitempty"`
	Deletes      []string        `json:"deletes,omitempty"`
	Files        []File          `json:"files"`
}

func invalid(field, message string) error { return &types.InputError{Field: field, Message: message} }
func tooLarge(field string, limit int64) error {
	return &types.InputError{Field: field, Message: fmt.Sprintf("exceeds limit of %d bytes", limit), TooLarge: true}
}

func readManifest(r io.Reader, limit int64) (Manifest, error) {
	var m Manifest
	body, err := io.ReadAll(io.LimitReader(r, ManifestLimit+1))
	if err != nil {
		return m, err
	}
	if len(body) > ManifestLimit {
		return m, tooLarge("/manifest", ManifestLimit)
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(body))); err != nil {
		return m, invalid("/manifest", "invalid or duplicate JSON members")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, invalid("/manifest", "invalid manifest: "+err.Error())
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return m, invalid("/manifest", "expected one JSON object")
	}
	return m, m.validate(limit)
}

func (m Manifest) validate(limit int64) error {
	if len(m.Files)+len(m.Deletes) > 100 {
		return &types.InputError{Field: "/files", Message: "at most 100 changes", TooLarge: true}
	}
	seen := make(map[string]bool)
	var total int64
	for i, f := range m.Files {
		field := fmt.Sprintf("/files/%d", i)
		if !partName.MatchString(f.Part) || f.Part == "manifest" || seen[f.Part] {
			return invalid(field+"/part", "expected a unique file part identifier")
		}
		seen[f.Part] = true
		if f.Mode != "" && f.Mode != "100644" && f.Mode != "100755" {
			return invalid(field+"/mode", "mode must be 100644 or 100755")
		}
		if f.Size == nil {
			continue
		}
		if *f.Size < 0 {
			return invalid(field+"/size", "size must be nonnegative")
		}
		if *f.Size > limit-total {
			return tooLarge(field+"/size", limit)
		}
		total += *f.Size
	}
	return nil
}

func (m Manifest) input(limit int64) types.CommitInput {
	return types.CommitInput{Branch: m.Branch, Message: m.Message, Author: m.Author, ExpectedHead: m.ExpectedHead,
		Base: m.Base, Replace: m.Replace, Deletes: m.Deletes, Multipart: true, ContentLimit: limit}
}

func uniqueJSON(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for dec.More() {
		if delim == '{' {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate member")
			}
			seen[name] = true
		}
		if err := uniqueJSON(dec); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}
