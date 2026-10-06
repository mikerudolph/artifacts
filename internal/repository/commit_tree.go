package repository

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

func validateCommit(repo types.Repo, input types.CommitInput) (string, error) {
	branch := input.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if _, err := types.ParseBranchName(branch); err != nil {
		return "", &types.InputError{Field: "/branch", Message: "invalid branch name"}
	}
	count := len(input.Files) + len(input.Deletes)
	if count > maxCommitFiles || (count == 0 && !input.Replace) {
		return "", &types.InputError{Field: "/files", Message: "commit must contain 1 to 100 file changes (or an explicit replacement)", TooLarge: count > maxCommitFiles}
	}
	if input.Replace && len(input.Deletes) != 0 {
		return "", &types.InputError{Field: "/deletes", Message: "replacement cannot also specify deletes"}
	}
	if err := validateCommitSHAs(input); err != nil {
		return "", err
	}
	return branch, validateCommitChanges(input)
}

func validateCommitChanges(input types.CommitInput) error {
	seen := map[string]bool{}
	total := int64(0)
	for i, file := range input.Files {
		if err := validateChangePath(file.Path, fmt.Sprintf("/files/%d/path", i), seen); err != nil {
			return err
		}
		if file.Mode != "" && file.Mode != "100644" && file.Mode != "100755" {
			return &types.InputError{Field: fmt.Sprintf("/files/%d/mode", i), Message: "mode must be 100644 or 100755"}
		}
		size := int64(len(file.Content))
		if file.Source != nil {
			size = file.Source.Size
		}
		if size < 0 || size > commitByteLimit(input)-total {
			return &types.InputError{Field: fmt.Sprintf("/files/%d", i), Message: "commit content exceeds byte limit", TooLarge: true}
		}
		total += size
	}
	for i, name := range input.Deletes {
		if err := validateChangePath(name, fmt.Sprintf("/deletes/%d", i), seen); err != nil {
			return err
		}
	}
	return nil
}

func commitByteLimit(input types.CommitInput) int64 {
	if input.Multipart && input.ContentLimit > 0 {
		return input.ContentLimit
	}
	return maxCommitBytes
}

func validateCommitSHAs(input types.CommitInput) error {
	for field, sha := range map[string]*string{"/expected_head": input.ExpectedHead, "/base": &input.Base} {
		if sha == nil || *sha == "" {
			continue
		}
		if b, err := hex.DecodeString(*sha); err != nil || len(b) != 20 || strings.ToLower(*sha) != *sha {
			return &types.InputError{Field: field, Message: "expected a full lowercase commit SHA"}
		}
	}
	return nil
}

func validateChangePath(name, field string, seen map[string]bool) error {
	invalid := name == "" || name == "." || name == ".." || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00\r\n")
	for _, part := range strings.Split(name, "/") {
		invalid = invalid || strings.EqualFold(part, ".git")
	}
	if invalid {
		return &types.InputError{Field: field, Message: "expected a normalized repository-relative file path"}
	}
	if seen[name] {
		return &types.InputError{Field: field, Message: "path occurs more than once in changes"}
	}
	seen[name] = true
	return nil
}

func writeCommit(ctx context.Context, gitDir, work, parent string, input types.CommitInput) (string, error) {
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(work, "index")}
	base := parent
	if input.Replace || base == "" {
		base = "--empty"
	}
	if _, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "read-tree", base); err != nil {
		return "", err
	}
	modes, err := readIndexModes(ctx, gitDir, env)
	if err != nil {
		return "", err
	}
	for _, name := range input.Deletes {
		if modes[name] == "" {
			continue
		}
		delete(modes, name)
		if _, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "--work-tree="+work, "update-index", "--force-remove", "--", name); err != nil {
			return "", err
		}
	}
	if err := validateTreeChanges(modes, input.Files); err != nil {
		return "", err
	}
	for _, file := range input.Files {
		blob, err := hashCommitFile(ctx, gitDir, file)
		if err != nil {
			return "", err
		}
		mode := modes[file.Path]
		if mode != "100755" {
			mode = "100644"
		}
		if file.Mode != "" {
			mode = file.Mode
		}
		if _, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "update-index", "--add", "--cacheinfo", mode, strings.TrimSpace(string(blob)), file.Path); err != nil {
			return "", err
		}
	}
	tree, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "write-tree")
	if err != nil {
		return "", err
	}
	return createCommit(ctx, gitDir, strings.TrimSpace(string(tree)), parent, input)
}

func hashCommitFile(ctx context.Context, gitDir string, file types.CommitFile) ([]byte, error) {
	var reader io.Reader = strings.NewReader(file.Content)
	if file.Source != nil {
		source, err := file.Source.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = source.Close() }()
		reader = source
	}
	return runGit(ctx, reader, "--git-dir="+gitDir, "hash-object", "-w", "--stdin")
}

func validateTreeChanges(modes map[string]string, files []types.CommitFile) error {
	for _, file := range files {
		if modes[file.Path] == "" {
			modes[file.Path] = "100644"
		}
	}
	for name := range modes {
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			if modes[dir] != "" {
				return &types.InputError{Field: "/files", Message: "file/directory collision: " + dir}
			}
		}
	}
	return nil
}

func readIndexModes(ctx context.Context, gitDir string, env []string) (map[string]string, error) {
	listing, err := runGitEnv(ctx, nil, env, "", "--git-dir="+gitDir, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	modes := map[string]string{}
	for _, line := range strings.Split(string(listing), "\x00") {
		meta, name, ok := strings.Cut(line, "\t")
		if ok {
			modes[name] = strings.Fields(meta)[0]
		}
	}
	return modes, nil
}
