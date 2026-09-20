package gitstore

import (
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage"
	"github.com/mikerudolph/artifacts/internal/store/meta"
)

const symPrefix = "ref:"

func (s *Store) SetReference(ref *plumbing.Reference) error {
	if ref == nil {
		return nil
	}
	oldVal := ""
	cur, err := s.refs.Get(s.ctx(), s.repo, string(ref.Name()))
	if err == nil {
		oldVal = cur.SHA
	} else if !meta.IsNotFound(err) {
		return err
	}
	return mapCAS(s.refs.CompareAndSwap(s.ctx(), s.repo, string(ref.Name()), oldVal, encodeRef(ref)))
}

func (s *Store) CheckAndSetReference(newRef, old *plumbing.Reference) error {
	if newRef == nil {
		return nil
	}
	if old == nil {
		return s.SetReference(newRef)
	}
	return mapCAS(s.refs.CompareAndSwap(s.ctx(), s.repo, string(newRef.Name()), encodeRef(old), encodeRef(newRef)))
}

func (s *Store) Reference(name plumbing.ReferenceName) (*plumbing.Reference, error) {
	got, err := s.refs.Get(s.ctx(), s.repo, string(name))
	if err != nil {
		if meta.IsNotFound(err) {
			return nil, plumbing.ErrReferenceNotFound
		}
		return nil, err
	}
	return decodeRef(got.Name, got.SHA), nil
}

func (s *Store) IterReferences() (storer.ReferenceIter, error) {
	all, err := s.refs.List(s.ctx(), s.repo)
	if err != nil {
		return nil, err
	}
	out := make([]*plumbing.Reference, 0, len(all))
	for _, r := range all {
		out = append(out, decodeRef(r.Name, r.SHA))
	}
	return storer.NewReferenceSliceIter(out), nil
}

func (s *Store) RemoveReference(name plumbing.ReferenceName) error {
	cur, err := s.refs.Get(s.ctx(), s.repo, string(name))
	if err != nil {
		if meta.IsNotFound(err) {
			return nil
		}
		return err
	}
	return mapCAS(s.refs.CompareAndSwap(s.ctx(), s.repo, string(name), cur.SHA, ""))
}

func (s *Store) CountLooseRefs() (int, error) {
	all, err := s.refs.List(s.ctx(), s.repo)
	if err != nil {
		return 0, err
	}
	return len(all), nil
}

func (s *Store) PackRefs() error { return nil }

func encodeRef(r *plumbing.Reference) string {
	if r.Type() == plumbing.SymbolicReference {
		return symPrefix + string(r.Target())
	}
	return r.Hash().String()
}

func decodeRef(name, value string) *plumbing.Reference {
	if strings.HasPrefix(value, symPrefix) {
		return plumbing.NewSymbolicReference(
			plumbing.ReferenceName(name),
			plumbing.ReferenceName(strings.TrimPrefix(value, symPrefix)),
		)
	}
	return plumbing.NewHashReference(plumbing.ReferenceName(name), plumbing.NewHash(value))
}

func mapCAS(err error) error {
	if meta.IsCASConflict(err) {
		return storage.ErrReferenceHasChanged
	}
	return err
}
