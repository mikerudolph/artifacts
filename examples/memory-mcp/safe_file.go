package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func writeRepositoryFile(root, relative string, data []byte) error {
	path, err := repositoryPath(root, relative)
	if err != nil {
		return err
	}
	if err := secureParents(root, filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("repository target is not a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return errors.New("inspect repository target")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errors.New("write repository file")
	}
	return nil
}

func readRepositoryFile(root, relative string) ([]byte, error) {
	path, err := repositoryPath(root, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("repository file not found")
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, errors.New("read repository file")
	}
	return data, nil
}

func repositoryPath(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid repository path")
	}
	return filepath.Join(root, clean), nil
}

func secureParents(root, parent string) error {
	relative, err := filepath.Rel(root, parent)
	if err != nil || strings.HasPrefix(relative, "..") {
		return errors.New("invalid repository parent")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o750); err != nil {
				return errors.New("create repository directory")
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe repository directory")
		}
	}
	return nil
}
