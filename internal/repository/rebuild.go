package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (m *Manager) installHistory(ctx context.Context, repo types.RepoID, through int64, path string) error {
	after := int64(0)
	line, err := m.meta.Forks().Get(ctx, repo)
	if err == nil {
		if err := m.installHistory(ctx, line.ParentRepoID, line.ParentSequence, path); err != nil {
			return err
		}
	} else if !meta.IsNotFound(err) {
		return err
	}
	cp, err := m.meta.Checkpoints().Get(ctx, repo)
	if err == nil && cp.Sequence <= through {
		if err := m.installPair(ctx, cp.PackKey, cp.IndexKey, cp.Checksum, path); err != nil {
			return err
		}
		after = cp.Sequence
	} else if err != nil && !meta.IsNotFound(err) {
		return err
	}
	packs, err := m.meta.WAL().List(ctx, repo, after, through)
	if err != nil {
		return err
	}
	for _, pack := range packs {
		if err := m.installPair(ctx, pack.PackKey, pack.IndexKey, pack.Checksum, path); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) installPair(ctx context.Context, packKey, indexKey, checksum, path string) error {
	packName := filepath.Base(packKey)
	indexName := filepath.Base(indexKey)
	if !strings.HasSuffix(packName, ".pack") || !strings.HasSuffix(indexName, ".idx") {
		return os.ErrInvalid
	}
	dir := filepath.Join(path, "objects", "pack")
	base := strings.TrimSuffix(packName, ".pack")
	if !strings.HasPrefix(base, "pack-") {
		base = "pack-" + base
	}
	packName, indexName = base+".pack", base+".idx"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := m.download(ctx, packKey, filepath.Join(dir, packName)); err != nil {
		return err
	}
	if checksum != "" {
		got, _, err := hashFile(filepath.Join(dir, packName))
		if err != nil || got != checksum {
			return errors.New("immutable pack checksum mismatch")
		}
	}
	return m.download(ctx, indexKey, filepath.Join(dir, indexName))
}

func (m *Manager) download(ctx context.Context, key, dest string) error {
	r, err := m.objects.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("get immutable object %s: %w", key, err)
	}
	defer func() { _ = r.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, dest)
}

func (m *Manager) installRefs(ctx context.Context, repo types.Repo, path string) error {
	refs, err := m.meta.Refs().List(ctx, repo.ID)
	if err != nil {
		return err
	}
	var script strings.Builder
	script.WriteString("start\n")
	for _, ref := range refs {
		if ref.Name == "HEAD" || strings.HasPrefix(ref.SHA, "ref:") {
			continue
		}
		script.WriteString("create " + ref.Name + " " + ref.SHA + "\n")
	}
	script.WriteString("prepare\ncommit\n")
	if _, err := runGit(ctx, strings.NewReader(script.String()), "--git-dir="+path, "update-ref", "--stdin"); err != nil {
		return err
	}
	_, err = runGit(ctx, nil, "--git-dir="+path, "symbolic-ref", "HEAD", "refs/heads/"+repo.DefaultBranch)
	return err
}
