package objecttest

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

// Mem is an in-memory Store used as the interface conformance reference.
type Mem struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewMem returns an empty memory-backed Store.
func NewMem() *Mem {
	return &Mem{data: make(map[string][]byte)}
}

// Get implements object.Store.
func (m *Mem) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if err := object.ValidateKey(key); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[key]
	if !ok {
		return nil, object.ErrNotFound
	}
	cp := append([]byte(nil), b...)
	return io.NopCloser(bytes.NewReader(cp)), nil
}

// Put implements object.Store.
func (m *Mem) Put(_ context.Context, key string, r io.Reader, _ int64) error {
	if err := object.ValidateKey(key); err != nil {
		return err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = b
	return nil
}

// Delete implements object.Store. Missing keys succeed.
func (m *Mem) Delete(_ context.Context, key string) error {
	if err := object.ValidateKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// DeletePrefix implements object.Store.
func (m *Mem) DeletePrefix(_ context.Context, prefix string) error {
	if prefix != "" {
		if err := object.ValidateKey(prefix); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			delete(m.data, k)
		}
	}
	return nil
}

// List implements object.Store.
func (m *Mem) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Copy implements object.Store.
func (m *Mem) Copy(_ context.Context, src, dst string) error {
	if err := object.ValidateKey(src); err != nil {
		return err
	}
	if err := object.ValidateKey(dst); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[src]
	if !ok {
		return object.ErrNotFound
	}
	m.data[dst] = append([]byte(nil), b...)
	return nil
}

// Exists implements object.Store.
func (m *Mem) Exists(_ context.Context, key string) (bool, error) {
	if err := object.ValidateKey(key); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

var _ object.Store = (*Mem)(nil)
