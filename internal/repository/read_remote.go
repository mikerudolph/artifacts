package repository

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/packread"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (m *Manager) ReadContent(ctx context.Context, repo types.Repo, visit func(storer.Storer) error) error {
	ranges, ok := m.objects.(object.RangeStore)
	if !ok || repo.StorageVersion == 1 {
		return m.Read(ctx, repo, visit)
	}
	path, unlock, err := m.lockedPath(repo)
	if err != nil {
		return err
	}
	defer unlock()
	repo, refs, err := m.contentSnapshot(ctx, repo)
	if err != nil {
		return err
	}
	history, err := m.packHistory(ctx, repo.ID, repo.WALSequence)
	if err != nil {
		return err
	}

	dir := path + ".indexes"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	packs := make([]packread.Pack, 0, len(history))
	for _, pack := range history {
		index, err := m.readIndex(ctx, pack, dir)
		if err != nil {
			return err
		}
		packs = append(packs, packread.Pack{Key: pack.PackKey, Index: index})
	}
	return visit(packread.New(ctx, ranges, packs, refs, m.diskObjectReader(ctx, repo, path, refs)))
}

func (m *Manager) contentSnapshot(ctx context.Context, repo types.Repo) (types.Repo, []*plumbing.Reference, error) {
	current, refs, err := m.snapshot(ctx, repo)
	if err != nil {
		return repo, nil, err
	}
	out := []*plumbing.Reference{plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(current.DefaultBranch))}
	for _, ref := range refs {
		if ref.Name != "HEAD" && !strings.HasPrefix(ref.SHA, "ref:") {
			out = append(out, plumbing.NewHashReference(plumbing.ReferenceName(ref.Name), plumbing.NewHash(ref.SHA)))
		}
	}
	return current, out, nil
}

func (m *Manager) packHistory(ctx context.Context, repo types.RepoID, through int64) ([]types.PackWAL, error) {
	after := int64(0)
	var out []types.PackWAL
	cp, err := m.meta.Checkpoints().Get(ctx, repo)
	if err == nil && cp.Sequence <= through {
		out = append(out, types.PackWAL{RepoID: repo, Sequence: cp.Sequence, PackKey: cp.PackKey, IndexKey: cp.IndexKey, Checksum: cp.Checksum})
		after = cp.Sequence
	} else {
		if err != nil && !meta.IsNotFound(err) {
			return nil, err
		}
		out, err = m.parentPacks(ctx, repo)
		if err != nil {
			return nil, err
		}
	}
	packs, err := m.meta.WAL().List(ctx, repo, after, through)
	return append(out, packs...), err
}

func (m *Manager) readIndex(ctx context.Context, pack types.PackWAL, dir string) (*idxfile.MemoryIndex, error) {
	path := filepath.Join(dir, filepath.Base(pack.IndexKey))
	index, err := decodeIndex(path, pack.PackKey)
	if err == nil {
		return index, nil
	}
	if err := m.download(ctx, pack.IndexKey, path); err != nil {
		return nil, err
	}
	index, err = decodeIndex(path, pack.PackKey)
	if err != nil {
		_ = os.Remove(path)
	}
	return index, err
}

func decodeIndex(path, key string) (*idxfile.MemoryIndex, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}
	if len(data) < 40 {
		return nil, errors.New("truncated pack index")
	}
	sum := sha1.Sum(data[:len(data)-20]) //nolint:gosec
	packHash := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(key), ".pack"), "pack-")
	if !bytes.Equal(sum[:], data[len(data)-20:]) || hex.EncodeToString(data[len(data)-40:len(data)-20]) != packHash {
		return nil, errors.New("pack index checksum mismatch")
	}
	index := &idxfile.MemoryIndex{}
	if err := idxfile.NewDecoder(bytes.NewReader(data)).Decode(index); err != nil {
		return nil, err
	}
	return index, nil
}

func (m *Manager) parentPacks(ctx context.Context, repo types.RepoID) ([]types.PackWAL, error) {
	line, err := m.meta.Forks().Get(ctx, repo)
	if meta.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m.packHistory(ctx, line.ParentRepoID, line.ParentSequence)
}

func (m *Manager) diskObjectReader(ctx context.Context, repo types.Repo, path string, refs []*plumbing.Reference) func(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error) {
	var disk storer.Storer
	return func(kind plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
		if disk == nil {
			var captured []types.Ref
			for _, ref := range refs {
				if ref.Type() == plumbing.HashReference {
					captured = append(captured, types.Ref{RepoID: repo.ID, Name: ref.Name().String(), SHA: ref.Hash().String()})
				}
			}
			if err := m.ensureSnapshot(ctx, repo, captured, path); err != nil {
				return nil, err
			}
			opened, err := gogit.PlainOpen(path)
			if err != nil {
				return nil, err
			}
			disk = opened.Storer
		}
		return disk.EncodedObject(kind, hash)
	}
}
