package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

const defaultReceiveLimit = int64(512 << 20)

func (m *Manager) Receive(ctx context.Context, repo types.Repo, input io.Reader, protocol string) ([]byte, error) {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return nil, err
	}
	defer unlock()
	repo, err = m.currentRepo(ctx, repo)
	if err != nil {
		return nil, err
	}
	if err := m.ensure(ctx, repo, path); err != nil {
		return nil, err
	}
	before, err := listRefs(ctx, path)
	if err != nil {
		return nil, err
	}
	staged, err := stageInput(path, input, defaultReceiveLimit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	response, err := runGitProtocol(ctx, staged, protocol, "-c", "gc.auto=0", "-c", "maintenance.auto=false", "receive-pack", "--stateless-rpc", path)
	_ = staged.Close()
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	after, err := listRefs(ctx, path)
	if err != nil {
		return nil, err
	}
	updates := refDiff(before, after)
	if len(updates) == 0 {
		return response, nil
	}
	pack, err := m.incrementalPack(ctx, repo, path, before, after)
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	_, err = m.meta.WAL().Publish(ctx, types.Publication{
		Pack: pack, Updates: updates, ExpectedSequence: repo.WALSequence,
	})
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	return response, writeCacheState(path, repo.WALSequence+1, repo.DefaultBranch)
}

func stageInput(path string, input io.Reader, limit int64) (*os.File, error) {
	f, err := os.CreateTemp(path, ".receive-*")
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(f, io.LimitReader(input, limit+1))
	if err != nil || n > limit {
		_ = f.Close()
		_ = os.Remove(f.Name())
		if err == nil {
			err = errors.New("receive pack exceeds limit")
		}
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func listRefs(ctx context.Context, path string) (map[string]string, error) {
	b, err := runGit(ctx, nil, "--git-dir="+path, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		name, sha, ok := strings.Cut(line, " ")
		if ok {
			out[name] = sha
		}
	}
	return out, nil
}

func refDiff(before, after map[string]string) []types.RefUpdate {
	names := make(map[string]struct{}, len(before)+len(after))
	for name := range before {
		names[name] = struct{}{}
	}
	for name := range after {
		names[name] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	var updates []types.RefUpdate
	for _, name := range ordered {
		if before[name] != after[name] {
			updates = append(updates, types.RefUpdate{Name: name, OldSHA: before[name], NewSHA: after[name]})
		}
	}
	return updates
}

func (m *Manager) packAndUpload(ctx context.Context, repo types.Repo, path string) (types.PackWAL, error) {
	objects, err := runGit(ctx, nil, "--git-dir="+path, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
	if err != nil {
		return types.PackWAL{}, err
	}
	if len(objects) == 0 {
		return types.PackWAL{}, errors.New("repository has no objects to compact")
	}
	return m.uploadPack(ctx, repo, path, string(objects), false)
}

func (m *Manager) incrementalPack(ctx context.Context, repo types.Repo, path string, before, after map[string]string) (types.PackWAL, error) {
	var revisions strings.Builder
	for _, sha := range after {
		revisions.WriteString(sha + "\n")
	}
	for _, sha := range before {
		revisions.WriteString("^" + sha + "\n")
	}
	return m.uploadPack(ctx, repo, path, revisions.String(), true)
}

func (m *Manager) uploadPack(ctx context.Context, repo types.Repo, path, input string, revisions bool) (types.PackWAL, error) {
	prefix := filepath.Join(path, "objects", "pack", "pack")
	args := []string{"--git-dir=" + path, "pack-objects", "--delta-base-offset"}
	if revisions {
		args = append(args, "--revs")
	}
	name, err := runGit(ctx, strings.NewReader(input), append(args, prefix)...)
	if err != nil {
		return types.PackWAL{}, err
	}
	packPath := prefix + "-" + strings.TrimSpace(string(name)) + ".pack"
	idxPath := strings.TrimSuffix(packPath, ".pack") + ".idx"
	checksum, size, err := hashFile(packPath)
	if err != nil {
		return types.PackWAL{}, err
	}
	packName := strings.TrimSuffix(filepath.Base(packPath), ".pack")
	packName = strings.TrimPrefix(packName, "pack-")
	packKey := object.PackKey(string(repo.AccountID), string(repo.ID), packName)
	idxKey := object.PackIndexKey(string(repo.AccountID), string(repo.ID), packName)
	if err := putFile(ctx, m.objects, packKey, packPath); err != nil {
		return types.PackWAL{}, err
	}
	if err := putFile(ctx, m.objects, idxKey, idxPath); err != nil {
		return types.PackWAL{}, err
	}
	return types.PackWAL{RepoID: repo.ID, PackKey: packKey, IndexKey: idxKey, Checksum: checksum, Size: size}, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func putFile(ctx context.Context, store object.Store, key, path string) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return store.Put(ctx, key, f, info.Size())
}
