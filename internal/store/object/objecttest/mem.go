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

type Mem struct {
	mu   sync.Mutex
	data map[string][]byte
}

func NewMem() *Mem {
	return &Mem{data: make(map[string][]byte)}
}

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
	if current, ok := m.data[key]; ok {
		if bytes.Equal(current, b) {
			return nil
		}
		return object.ErrImmutableConflict
	}
	m.data[key] = b
	return nil
}

func (m *Mem) Delete(_ context.Context, key string) error {
	if err := object.ValidateKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

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
	if current, exists := m.data[dst]; exists {
		if bytes.Equal(current, b) {
			return nil
		}
		return object.ErrImmutableConflict
	}
	m.data[dst] = append([]byte(nil), b...)
	return nil
}

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

func (m *Mem) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length <= 0 {
		return nil, object.ErrInvalidKey
	}
	r, err := m.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if _, err := io.CopyN(io.Discard, r, offset); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(r, length))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
