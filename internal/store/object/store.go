package object

import (
	"context"
	"errors"
	"io"
)

// Store is a content-addressed object backend (filesystem or S3-compatible).
type Store interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Copy(ctx context.Context, src, dst string) error
	Exists(ctx context.Context, key string) (bool, error)
}

var (
	// ErrNotFound means the key does not exist.
	ErrNotFound = errors.New("object not found")
	// ErrInvalidKey means the key is empty or not a relative path.
	ErrInvalidKey = errors.New("invalid object key")
	// ErrImmutableConflict means an existing immutable key has different bytes.
	ErrImmutableConflict = errors.New("immutable object conflict")
)

// IsNotFound reports whether err is ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
