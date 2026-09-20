package object

import (
	"context"
	"errors"
	"io"
)

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
	ErrNotFound = errors.New("object not found")

	ErrInvalidKey = errors.New("invalid object key")

	ErrImmutableConflict = errors.New("immutable object conflict")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
