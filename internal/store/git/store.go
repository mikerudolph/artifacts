package gitstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/types"
)

type Store struct {
	objects object.Store
	refs    meta.Refs
	account types.AccountID
	repo    types.RepoID
}

func Open(objects object.Store, refs meta.Refs, account types.AccountID, repo types.RepoID) (storer.Storer, error) {
	if objects == nil || refs == nil {
		return nil, errors.New("objects and refs are required")
	}
	if account == "" || repo == "" {
		return nil, errors.New("account and repo are required")
	}
	return &Store{objects: objects, refs: refs, account: account, repo: repo}, nil
}

func (s *Store) NewEncodedObject() plumbing.EncodedObject {
	return &plumbing.MemoryObject{}
}

func (s *Store) AddAlternate(string) error {
	return errNotSupported
}

func (s *Store) ctx() context.Context { return context.Background() }

func (s *Store) looseKey(h plumbing.Hash) string {
	return object.LooseObjectKey(string(s.account), string(s.repo), h.String())
}

func (s *Store) objectsPrefix() string {
	return object.ObjectsPrefix(string(s.account), string(s.repo)) + "/objects/"
}

var errNotSupported = fmt.Errorf("not supported")

var _ storer.Storer = (*Store)(nil)
