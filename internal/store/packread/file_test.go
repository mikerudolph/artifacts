package packread

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
)

func TestRangeFileBoundariesAndBoundedCache(t *testing.T) {
	ctx := context.Background()
	objects := objecttest.NewMem()
	data := bytes.Repeat([]byte("0123456789"), 300000)
	if err := objects.Put(ctx, "pack", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatal(err)
	}
	f := &rangeFile{ctx: ctx, store: objects, key: "pack"}
	for _, offset := range []int64{0, blockSize - 3, blockSize + 3, int64(len(data)) - 3} {
		b := make([]byte, 10)
		n, err := f.ReadAt(b, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if !bytes.Equal(b[:n], data[offset:offset+int64(n)]) {
			t.Fatal("incorrect boundary read")
		}
	}
	for offset := int64(0); offset < int64(len(data)); offset += blockSize {
		if _, err := f.ReadAt(make([]byte, 1), offset); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.blocks) != 8 {
		t.Fatalf("unbounded cache %d", len(f.blocks))
	}
	if _, err := f.Seek(3, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if n, err := f.Seek(2, io.SeekCurrent); err != nil || n != 5 {
		t.Fatal(n, err)
	}
	b := make([]byte, 3)
	if _, err := f.Read(b); err != nil || string(b) != "567" {
		t.Fatal(string(b), err)
	}
	testInvalidFileOperations(t, f, int64(len(data)))
}

func testInvalidFileOperations(t *testing.T, f *rangeFile, size int64) {
	t.Helper()
	b := make([]byte, 3)
	ctx := context.Background()
	if _, err := f.ReadAt(b, size+blockSize); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if _, err := f.ReadAt(b, -1); !errors.Is(err, os.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := f.Seek(-1, io.SeekStart); err == nil {
		t.Fatal("negative seek")
	}
	if _, err := f.Seek(0, io.SeekEnd); err == nil {
		t.Fatal("unsupported seek")
	}
	if _, err := f.Write(b); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	if err := f.Truncate(0); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	if f.Name() != "pack" || f.Close() != nil || f.Lock() != nil || f.Unlock() != nil {
		t.Fatal("file contract")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	f.ctx = canceled
	if _, err := f.Read(b); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
