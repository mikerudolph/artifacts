package types

import (
	"fmt"
	"strings"
	"unicode"
)

const maxNameLen = 100

type NamespaceName string

type RepoName string

type BranchName string

func ParseNamespaceName(s string) (NamespaceName, error) {
	if err := validateName(s); err != nil {
		return "", fmt.Errorf("namespace name: %w", err)
	}
	return NamespaceName(s), nil
}

func ParseRepoName(s string) (RepoName, error) {
	if err := validateName(s); err != nil {
		return "", fmt.Errorf("repo name: %w", err)
	}
	return RepoName(s), nil
}

func ParseBranchName(s string) (BranchName, error) {
	if s == "" {
		return "", fmt.Errorf("branch name: %w", ErrInvalidName)
	}
	if len(s) > maxNameLen {
		return "", fmt.Errorf("branch name: %w", ErrInvalidName)
	}
	if s == "HEAD" || s[0] == '-' || s[len(s)-1] == '.' ||
		strings.ContainsAny(s, " ~^:?*[\\") || strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.Contains(s, "//") {
		return "", fmt.Errorf("branch name: %w", ErrInvalidName)
	}
	if !validRefComponents(s) {
		return "", fmt.Errorf("branch name: %w", ErrInvalidName)
	}
	return BranchName(s), nil
}

func validRefComponents(s string) bool {
	for _, part := range strings.Split(s, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return s != "@"
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
