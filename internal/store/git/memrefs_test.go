package gitstore

import (
	"context"
	"sync"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type memRefs struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemRefs() *memRefs {
	return &memRefs{data: make(map[string]string)}
}

func (m *memRefs) Get(_ context.Context, _ types.RepoID, name string) (types.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sha, ok := m.data[name]
	if !ok {
		return types.Ref{}, meta.ErrNotFound
	}
	return types.Ref{Name: name, SHA: sha}, nil
}

func (m *memRefs) List(_ context.Context, _ types.RepoID) ([]types.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]types.Ref, 0, len(m.data))
	for n, sha := range m.data {
		out = append(out, types.Ref{Name: n, SHA: sha})
	}
	return out, nil
}

func (m *memRefs) CompareAndSwap(_ context.Context, repo types.RepoID, name, oldSHA, newSHA string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.data[name]
	switch {
	case oldSHA == "" && newSHA == "":
		return nil
	case oldSHA == "":
		if ok {
			return meta.ErrCASConflict
		}
		m.data[name] = newSHA
	case newSHA == "":
		if !ok || cur != oldSHA {
			return meta.ErrCASConflict
		}
		delete(m.data, name)
	default:
		if !ok || cur != oldSHA {
			return meta.ErrCASConflict
		}
		m.data[name] = newSHA
	}
	return nil
}

func (m *memRefs) DeleteAll(_ context.Context, _ types.RepoID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]string)
	return nil
}
