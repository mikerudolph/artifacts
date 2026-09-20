package fs

import (
	"context"
	"io"
	"os"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

func (s *store) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length <= 0 {
		return nil, os.ErrInvalid
	}
	r, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	f := r.(*os.File)
	info, err := f.Stat()
	if err == nil && offset >= info.Size() {
		err = io.EOF
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &rangeReadCloser{Reader: io.NewSectionReader(f, offset, length), Closer: f}, nil
}

type rangeReadCloser struct {
	io.Reader
	io.Closer
}

var _ object.RangeStore = (*store)(nil)
