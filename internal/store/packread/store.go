package packread

import (
	"context"
	"fmt"
	"io"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

type Pack struct {
	Key   string
	Index *idxfile.MemoryIndex
}

type store struct {
	*memory.Storage
	ctx      context.Context
	objects  object.RangeStore
	packs    []Pack
	file     *rangeFile
	fallback func(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error)
}

func New(ctx context.Context, objects object.RangeStore, packs []Pack, refs []*plumbing.Reference, fallback func(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error)) storer.Storer {
	s := &store{Storage: memory.NewStorage(), ctx: ctx, objects: objects, packs: packs, fallback: fallback}
	for _, ref := range refs {
		_ = s.SetReference(ref)
	}
	return s
}

func (s *store) EncodedObject(kind plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	for i := len(s.packs) - 1; i >= 0; i-- {
		p := s.packs[i]
		if _, err := p.Index.FindOffset(hash); err != nil {
			continue
		}
		if s.file == nil || s.file.key != p.Key {
			s.file = &rangeFile{ctx: s.ctx, store: s.objects, key: p.Key}
		}
		s.file.offset = 0
		pack := packfile.NewPackfile(p.Index, nil, s.file, 0)
		obj, err := s.decodeObject(pack, kind, hash)
		_ = pack.Close()
		if err != nil {
			return nil, err
		}
		if obj.Hash() != hash {
			return nil, fmt.Errorf("immutable Git object hash mismatch")
		}
		if kind != plumbing.AnyObject && obj.Type() != kind {
			return nil, plumbing.ErrObjectNotFound
		}
		return obj, nil
	}
	return nil, plumbing.ErrObjectNotFound
}
func (s *store) HasEncodedObject(hash plumbing.Hash) error {
	for _, p := range s.packs {
		if _, err := p.Index.FindOffset(hash); err == nil {
			return nil
		}
	}
	return plumbing.ErrObjectNotFound
}
func (s *store) EncodedObjectSize(hash plumbing.Hash) (int64, error) {
	obj, err := s.EncodedObject(plumbing.AnyObject, hash)
	if err != nil {
		return 0, err
	}
	return obj.Size(), nil
}
func (s *store) IterEncodedObjects(kind plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	hashes := []plumbing.Hash{}
	seen := map[plumbing.Hash]bool{}
	for _, p := range s.packs {
		iter, err := p.Index.Entries()
		if err != nil {
			return nil, err
		}
		for {
			entry, err := iter.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = iter.Close()
				return nil, err
			}
			if !seen[entry.Hash] {
				seen[entry.Hash] = true
				hashes = append(hashes, entry.Hash)
			}
		}
		_ = iter.Close()
	}
	return storer.NewEncodedObjectLookupIter(s, kind, hashes), nil
}

func (s *store) decodeObject(pack *packfile.Packfile, kind plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	offset, err := pack.FindOffset(hash)
	if err != nil {
		return nil, err
	}
	size, err := pack.GetSizeByOffset(offset)
	if err != nil {
		return nil, err
	}
	if size > 8<<20 && s.fallback != nil {
		return s.fallback(kind, hash)
	}
	return pack.Get(hash)
}
