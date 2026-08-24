package types

import (
	"fmt"
	"unicode"
)

const maxNameLen = 100

// NamespaceName is unique within an account.
type NamespaceName string

// RepoName is unique within a namespace.
type RepoName string

// BranchName is a git branch name.
type BranchName string

// ParseNamespaceName validates a Cloudflare Artifacts namespace name.
func ParseNamespaceName(s string) (NamespaceName, error) {
	if err := validateName(s); err != nil {
		return "", fmt.Errorf("namespace name: %w", err)
	}
	return NamespaceName(s), nil
}

// ParseRepoName validates a Cloudflare Artifacts repository name.
func ParseRepoName(s string) (RepoName, error) {
	if err := validateName(s); err != nil {
		return "", fmt.Errorf("repo name: %w", err)
	}
	return RepoName(s), nil
}

// ParseBranchName validates a non-empty branch name.
func ParseBranchName(s string) (BranchName, error) {
	if s == "" {
		return "", fmt.Errorf("branch name: %w", ErrInvalidName)
	}
	if len(s) > maxNameLen {
		return "", fmt.Errorf("branch name: %w", ErrInvalidName)
	}
	return BranchName(s), nil
}

func validateName(s string) error {
	if s == "" || len(s) > maxNameLen {
		return ErrInvalidName
	}
	r := []rune(s)
	if !unicode.IsLetter(r[0]) && !unicode.IsDigit(r[0]) {
		return ErrInvalidName
	}
	for _, c := range r[1:] {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '.' || c == '_' || c == '-' {
			continue
		}
		return ErrInvalidName
	}
	return nil
}
