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

// Receive applies a bounded receive-pack, uploads its immutable pack, then atomically publishes refs.
func (m *Manager) Receive(ctx context.Context, repo types.Repo, input io.Reader, protocol string) ([]byte, error) {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return nil, err
	}
	defer unlock()
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
	response, err := runGitProtocol(ctx, staged, protocol, "receive-pack", "--stateless-rpc", path)
	_ = staged.Close()
	if err != nil {
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
	pack, err := m.packAndUpload(ctx, repo, path)
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
	if _, err := runGit(ctx, nil, "--git-dir="+path, "repack", "-a", "-d"); err != nil {
		return types.PackWAL{}, err
	}
	matches, err := filepath.Glob(filepath.Join(path, "objects", "pack", "*.pack"))
	if err != nil || len(matches) != 1 {
		return types.PackWAL{}, errors.New("expected one repository pack")
	}
	packPath := matches[0]
	idxPath := strings.TrimSuffix(packPath, ".pack") + ".idx"
	checksum, size, err := hashFile(packPath)
	if err != nil {
		return types.PackWAL{}, err
	}
	name := strings.TrimSuffix(filepath.Base(packPath), ".pack")
	name = strings.TrimPrefix(name, "pack-")
	packKey := object.PackKey(string(repo.AccountID), string(repo.ID), name)
	idxKey := object.PackIndexKey(string(repo.AccountID), string(repo.ID), name)
	if err := putFile(ctx, m.objects, packKey, packPath); err != nil {
		return types.PackWAL{}, err
	}
	if err := putFile(ctx, m.objects, idxKey, idxPath); err != nil {
		return types.PackWAL{}, err
	}
	return types.PackWAL{RepoID: repo.ID, PackKey: packKey, IndexKey: idxKey, Checksum: checksum, Size: size}, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path) //nolint:gosec // controlled cache path
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
	f, err := os.Open(path) //nolint:gosec // controlled cache path
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
