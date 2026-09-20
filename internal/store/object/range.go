package object

import (
	"context"
	"io"
)

type RangeStore interface {
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
}
