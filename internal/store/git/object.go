package gitstore

import (
	"bytes"
	"io"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/objfile"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

func (s *Store) SetEncodedObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	h := obj.Hash()
	raw, err := encodeObject(obj)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	err = s.objects.Put(s.ctx(), s.looseKey(h), bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return h, nil
}

func (s *Store) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	obj, err := s.readObject(h)
	if err != nil {
		return nil, err
	}
	if t != plumbing.AnyObject && obj.Type() != t {
		return nil, plumbing.ErrObjectNotFound
	}
	return obj, nil
}

func (s *Store) HasEncodedObject(h plumbing.Hash) error {
	ok, err := s.objects.Exists(s.ctx(), s.looseKey(h))
	if err != nil {
		return err
	}
	if !ok {
		return plumbing.ErrObjectNotFound
	}
	return nil
}

func (s *Store) EncodedObjectSize(h plumbing.Hash) (int64, error) {
	obj, err := s.readObject(h)
	if err != nil {
		return 0, err
	}
	return obj.Size(), nil
}

func (s *Store) IterEncodedObjects(t plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	keys, err := s.objects.List(s.ctx(), s.objectsPrefix())
	if err != nil {
		return nil, err
	}
	var series []plumbing.EncodedObject
	for _, key := range keys {
		if strings.HasSuffix(key, ".pack") || strings.HasSuffix(key, ".idx") {
			continue
		}
		h := plumbing.NewHash(shaFromLooseKey(key))
		if h.IsZero() {
			continue
		}
		obj, err := s.EncodedObject(t, h)
		if err != nil {
			if err == plumbing.ErrObjectNotFound {
				continue
			}
			return nil, err
		}
		series = append(series, obj)
	}
	return storer.NewEncodedObjectSliceIter(series), nil
}

func (s *Store) readObject(h plumbing.Hash) (plumbing.EncodedObject, error) {
	rc, err := s.objects.Get(s.ctx(), s.looseKey(h))
	if err != nil {
		if object.IsNotFound(err) {
			return nil, plumbing.ErrObjectNotFound
		}
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return decodeObject(rc)
}

func encodeObject(obj plumbing.EncodedObject) ([]byte, error) {
	var buf bytes.Buffer
	w := objfile.NewWriter(&buf)
	if err := w.WriteHeader(obj.Type(), obj.Size()); err != nil {
		return nil, err
	}
	r, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if _, err := io.Copy(w, r); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeObject(r io.Reader) (plumbing.EncodedObject, error) {
	or, err := objfile.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = or.Close() }()
	t, size, err := or.Header()
	if err != nil {
		return nil, err
	}
	obj := &plumbing.MemoryObject{}
	obj.SetType(t)
	obj.SetSize(size)
	if _, err := io.Copy(obj, or); err != nil {
		return nil, err
	}
	return obj, nil
}

func shaFromLooseKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + parts[len(parts)-1]
}
