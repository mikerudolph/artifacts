package packread

import (
	"context"
	"io"
	"os"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

const blockSize = int64(256 << 10)

type rangeFile struct {
	ctx    context.Context
	store  object.RangeStore
	key    string
	offset int64
	blocks map[int64][]byte
	order  []int64
}

func (f *rangeFile) Name() string              { return f.key }
func (f *rangeFile) Close() error              { return nil }
func (f *rangeFile) Lock() error               { return nil }
func (f *rangeFile) Unlock() error             { return nil }
func (f *rangeFile) Write([]byte) (int, error) { return 0, os.ErrPermission }
func (f *rangeFile) Truncate(int64) error      { return os.ErrPermission }
func (f *rangeFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.offset
	default:
		return 0, os.ErrInvalid
	}
	if offset < 0 {
		return 0, os.ErrInvalid
	}
	f.offset = offset
	return offset, nil
}
func (f *rangeFile) Read(p []byte) (int, error) {
	n, err := f.ReadAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}
func (f *rangeFile) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, os.ErrInvalid
	}
	n := 0
	for len(p) > 0 {
		start := offset / blockSize * blockSize
		block, err := f.block(start)
		if err != nil {
			return n, err
		}
		pos := offset - start
		if pos >= int64(len(block)) {
			return n, io.EOF
		}
		copied := copy(p, block[pos:])
		n += copied
		offset += int64(copied)
		p = p[copied:]
	}
	return n, nil
}
func (f *rangeFile) block(start int64) ([]byte, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	if b, ok := f.blocks[start]; ok {
		return b, nil
	}
	r, err := f.store.GetRange(f.ctx, f.key, start, blockSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(io.LimitReader(r, blockSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > blockSize {
		return nil, os.ErrInvalid
	}
	if f.blocks == nil {
		f.blocks = map[int64][]byte{}
	}
	if len(f.order) == 8 {
		delete(f.blocks, f.order[0])
		f.order = f.order[1:]
	}
	f.blocks[start] = b
	f.order = append(f.order, start)
	return b, nil
}
