package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryFilesRejectUnsafePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeRepositoryFile(root, "a/b.txt", []byte("value")); err != nil {
		t.Fatal(err)
	}
	data, err := readRepositoryFile(root, "a/b.txt")
	if err != nil || string(data) != "value" {
		t.Fatalf("read=%q error=%v", data, err)
	}
	for _, path := range []string{".", "..", "../escape", "/absolute"} {
		if err := writeRepositoryFile(root, path, []byte("x")); err == nil {
			t.Errorf("unsafe write accepted: %s", path)
		}
		if _, err := readRepositoryFile(root, path); err == nil {
			t.Errorf("unsafe read accepted: %s", path)
		}
	}
	if _, err := readRepositoryFile(root, "missing"); err == nil {
		t.Fatal("missing file read")
	}
}

func TestRepositoryFilesRejectSymlinksAndSpecialFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := writeRepositoryFile(root, "link/file", []byte("x")); err == nil {
		t.Fatal("symlink parent accepted")
	}
	if err := writeRepositoryFile(root, "link", []byte("x")); err == nil {
		t.Fatal("symlink target accepted")
	}
	if _, err := readRepositoryFile(root, "link"); err == nil {
		t.Fatal("symlink read accepted")
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeRepositoryFile(root, "directory", []byte("x")); err == nil {
		t.Fatal("directory target accepted")
	}
}
