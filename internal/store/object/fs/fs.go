package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

type store struct {
	root string
}

// New returns a filesystem-backed object.Store rooted at root.
func New(root string) (object.Store, error) {
	if root == "" {
		return nil, fmt.Errorf("empty root")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &store{root: abs}, nil
}

func (s *store) resolve(key string) (string, error) {
	if err := object.ValidateKey(key); err != nil {
		return "", err
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}

func (s *store) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Clean(path)) //nolint:gosec // path is ValidateKey'd
	if err != nil {
		if os.IsNotExist(err) {
			return nil, object.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (s *store) Put(_ context.Context, key string, r io.Reader, _ int64) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.Create(filepath.Clean(path)) //nolint:gosec // path is ValidateKey'd
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, r)
	return err
}

func (s *store) Delete(_ context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *store) Exists(_ context.Context, key string) (bool, error) {
	path, err := s.resolve(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *store) Copy(ctx context.Context, src, dst string) error {
	rc, err := s.Get(ctx, src)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return s.Put(ctx, dst, rc, -1)
}

func (s *store) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *store) DeletePrefix(ctx context.Context, prefix string) error {
	keys, err := s.List(ctx, prefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := s.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
