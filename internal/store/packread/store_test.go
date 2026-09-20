package packread

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
)

func packedFixture(t *testing.T) (*objecttest.Mem, Pack, []plumbing.Hash) {
	t.Helper()
	dir := t.TempDir()
	git := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(input)
		b, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	git("", "init", "--bare")
	var hashes []plumbing.Hash
	var input strings.Builder
	for _, ending := range []string{"first", "second", "third"} {
		hash := git(strings.Repeat("common content\n", 2000)+ending, "hash-object", "-w", "--stdin")
		hashes = append(hashes, plumbing.NewHash(hash))
		input.WriteString(hash + "\n")
	}
	name := git(input.String(), "pack-objects", "--delta-base-offset", "--window=10", filepath.Join(dir, "pack"))
	raw, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "pack-"+name+".pack")))
	if err != nil {
		t.Fatal(err)
	}
	indexData, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "pack-"+name+".idx")))
	if err != nil {
		t.Fatal(err)
	}
	index := &idxfile.MemoryIndex{}
	if err := idxfile.NewDecoder(bytes.NewReader(indexData)).Decode(index); err != nil {
		t.Fatal(err)
	}
	objects := objecttest.NewMem()
	if err := objects.Put(context.Background(), "pack", bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatal(err)
	}
	return objects, Pack{Key: "pack", Index: index}, hashes
}

func TestIndexedObjectsAndDeltas(t *testing.T) {
	objects, pack, hashes := packedFixture(t)
	s := New(context.Background(), objects, []Pack{pack, pack}, []*plumbing.Reference{plumbing.NewHashReference("refs/heads/main", hashes[0])}, nil)
	if _, err := s.Reference("refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	for _, hash := range hashes {
		obj, err := s.EncodedObject(plumbing.BlobObject, hash)
		if err != nil {
			t.Fatal(err)
		}
		if obj.Hash() != hash {
			t.Fatal("decoded delta hash mismatch")
		}
		size, err := s.EncodedObjectSize(hash)
		if err != nil || size != obj.Size() {
			t.Fatal(size, err)
		}
		if err := s.HasEncodedObject(hash); err != nil {
			t.Fatal(err)
		}
	}
	iter, err := s.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		_, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	iter.Close()
	if count != 3 {
		t.Fatalf("duplicate objects in iterator: %d", count)
	}
	testMissingObjects(t, objects, pack, hashes)
}

func testMissingObjects(t *testing.T, objects *objecttest.Mem, pack Pack, hashes []plumbing.Hash) {
	t.Helper()
	s := New(context.Background(), objects, []Pack{pack}, nil, nil)
	if _, err := s.EncodedObject(plumbing.CommitObject, hashes[0]); !errors.Is(err, plumbing.ErrObjectNotFound) {
		t.Fatal(err)
	}
	if err := s.HasEncodedObject(plumbing.ZeroHash); !errors.Is(err, plumbing.ErrObjectNotFound) {
		t.Fatal(err)
	}
	if _, err := s.EncodedObjectSize(plumbing.ZeroHash); !errors.Is(err, plumbing.ErrObjectNotFound) {
		t.Fatal(err)
	}
	if err := objects.Delete(context.Background(), "pack"); err != nil {
		t.Fatal(err)
	}
	fresh := New(context.Background(), objects, []Pack{pack}, nil, nil)
	if _, err := fresh.EncodedObject(plumbing.AnyObject, hashes[0]); err == nil {
		t.Fatal("missing pack accepted")
	}
}
