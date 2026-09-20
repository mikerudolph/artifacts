package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

type Manager struct {
	meta    meta.V2Store
	objects object.Store
	root    string
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
}

func New(metadata meta.V2Store, objects object.Store, root string) (*Manager, error) {
	if metadata == nil || objects == nil || root == "" {
		return nil, errors.New("metadata, objects, and cache root are required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, err
	}
	return &Manager{meta: metadata, objects: objects, root: abs, locks: map[string]*sync.Mutex{}}, nil
}

func (m *Manager) Upgrade(ctx context.Context, repo types.Repo) (types.Repo, error) {
	if repo.StorageVersion != 1 {
		return repo, nil
	}
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return types.Repo{}, err
	}
	defer unlock()
	if err := m.ensure(ctx, repo, path); err != nil {
		return types.Repo{}, err
	}
	return m.meta.Repos().GetByID(ctx, repo.ID)
}

func (m *Manager) Read(ctx context.Context, repo types.Repo, visit func(storer.Storer) error) error {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.ensure(ctx, repo, path); err != nil {
		return err
	}
	r, err := gogit.PlainOpen(path)
	if err != nil {
		return err
	}
	return visit(r.Storer)
}

func (m *Manager) Evict(repo types.Repo) error {
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return err
	}
	defer unlock()
	return os.RemoveAll(path)
}

func (m *Manager) lockedPath(repo types.Repo) (string, func(), error) {
	if repo.AccountID == "" || repo.ID == "" {
		return "", nil, errors.New("account and repository id are required")
	}
	key := string(repo.AccountID) + "/" + string(repo.ID)
	path := filepath.Join(m.root, filepath.FromSlash(key))
	if !within(m.root, path) {
		return "", nil, errors.New("invalid cache path")
	}
	m.mu.Lock()
	lock := m.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[key] = lock
	}
	m.mu.Unlock()
	lock.Lock()
	lockFile, err := m.lockFile(key)
	if err != nil {
		lock.Unlock()
		return "", nil, err
	}
	unlock := func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		lock.Unlock()
	}
	return path, unlock, nil
}

func (m *Manager) lockFile(key string) (*os.File, error) {
	name := filepath.Join(m.root, ".locks", filepath.FromSlash(key)+".lock")
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel)
}

func (m *Manager) ensure(ctx context.Context, repo types.Repo, path string) error {
	if repo.StorageVersion == 1 {
		current, err := m.meta.Repos().GetByID(ctx, repo.ID)
		if err != nil {
			return err
		}
		repo = current
	}
	sequence, branch, err := readCacheState(path)
	if err == nil && sequence == repo.WALSequence && branch == repo.DefaultBranch && gitHealthy(ctx, path) {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if _, err := runGit(ctx, nil, "init", "--bare", "--initial-branch="+repo.DefaultBranch, path); err != nil {
		return err
	}
	if repo.StorageVersion == 1 {
		return m.convertLegacy(ctx, repo, path)
	}
	if err := m.installHistory(ctx, repo.ID, repo.WALSequence, path); err != nil {
		_ = os.RemoveAll(path)
		return err
	}
	if err := m.installRefs(ctx, repo, path); err != nil {
		_ = os.RemoveAll(path)
		return err
	}
	return writeCacheState(path, repo.WALSequence, repo.DefaultBranch)
}

func (m *Manager) convertLegacy(ctx context.Context, repo types.Repo, path string) error {
	legacy, err := gitstore.Open(m.objects, m.meta.Refs(), repo.AccountID, repo.ID)
	if err != nil {
		return err
	}
	disk, err := gogit.PlainOpen(path)
	if err != nil {
		return err
	}
	objects, err := legacy.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return err
	}
	defer objects.Close()
	if err := objects.ForEach(func(obj plumbing.EncodedObject) error {
		_, err := disk.Storer.SetEncodedObject(obj)
		return err
	}); err != nil {
		return err
	}
	if err := m.installRefs(ctx, repo, path); err != nil {
		return err
	}
	pack, err := m.packAndUpload(ctx, repo, path)
	if err != nil {
		return err
	}
	sequence, err := m.meta.WAL().Publish(ctx, types.Publication{
		Pack: pack, ExpectedSequence: repo.WALSequence, AllowReadOnly: true, UpgradeFrom: 1,
	})
	if err != nil {
		return err
	}
	return writeCacheState(path, sequence, repo.DefaultBranch)
}

func readCacheState(path string) (int64, string, error) {
	b, err := os.ReadFile(filepath.Join(path, ".artifacts-sequence")) //nolint:gosec
	if err != nil {
		return 0, "", err
	}
	var sequence int64
	var branch string
	_, err = fmt.Sscanf(string(b), "%d %s", &sequence, &branch)
	return sequence, branch, err
}

func writeCacheState(path string, sequence int64, branch string) error {
	tmp, err := os.CreateTemp(path, ".sequence-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := fmt.Fprintf(tmp, "%d %s", sequence, branch); err != nil {
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
	return os.Rename(name, filepath.Join(path, ".artifacts-sequence"))
}

func gitHealthy(ctx context.Context, path string) bool {
	_, err := runGit(ctx, nil, "--git-dir="+path, "fsck", "--connectivity-only", "--no-dangling")
	return err == nil
}
