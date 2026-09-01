package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const maxOutputBytes = 8 << 20

var memoryKinds = map[string]bool{
	"note": true, "fact": true, "event": true, "goal": true,
	"decision": true, "preference": true,
}

var recallTypes = map[string]bool{
	"memory": true, "thread": true, "run": true, "output": true,
}

type temporal struct {
	StartsAt string `json:"starts_at,omitempty" yaml:"starts_at,omitempty"`
	EndsAt   string `json:"ends_at,omitempty" yaml:"ends_at,omitempty"`
	DueAt    string `json:"due_at,omitempty" yaml:"due_at,omitempty"`
	Timezone string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

func validateTemporal(value temporal) error {
	start, err := optionalTime(value.StartsAt)
	if err != nil {
		return fmt.Errorf("starts_at: %w", err)
	}
	end, err := optionalTime(value.EndsAt)
	if err != nil {
		return fmt.Errorf("ends_at: %w", err)
	}
	if _, err := optionalTime(value.DueAt); err != nil {
		return fmt.Errorf("due_at: %w", err)
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return errors.New("ends_at precedes starts_at")
	}
	if value.Timezone != "" {
		if _, err := time.LoadLocation(value.Timezone); err != nil {
			return errors.New("timezone is not an IANA timezone")
		}
	}
	return nil
}

func optionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("must be RFC3339")
	}
	return parsed, nil
}

func normalizeTags(tags []string) ([]string, error) {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			return nil, errors.New("tags must be non-empty")
		}
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out, nil
}

func validateEnum(value, label string, allowed map[string]bool) error {
	if !allowed[value] {
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func validateID(value, prefix string) error {
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) {
		return fmt.Errorf("invalid %s id", strings.TrimSuffix(prefix, "_"))
	}
	for _, r := range value[len(prefix):] {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("invalid %s id", strings.TrimSuffix(prefix, "_"))
		}
	}
	return nil
}

func validateOutputName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return errors.New("output name must be a plain filename")
	}
	if strings.EqualFold(name, ".git") {
		return errors.New("output name is Git-reserved")
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return errors.New("output name contains unsafe characters")
		}
	}
	return nil
}

func decodeOutput(content, encoding string) ([]byte, string, error) {
	if encoding == "" || encoding == "utf-8" {
		if len(content) > maxOutputBytes {
			return nil, "", errors.New("output exceeds 8 MiB")
		}
		return []byte(content), "utf-8", nil
	}
	if encoding != "base64" {
		return nil, "", errors.New("encoding must be utf-8 or base64")
	}
	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, "", errors.New("content is not valid base64")
	}
	if len(data) > maxOutputBytes {
		return nil, "", errors.New("output exceeds 8 MiB")
	}
	return data, encoding, nil
}
