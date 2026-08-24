package gitstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

// PackfileWriter implements storer.PackfileWriter.
func (s *Store) PackfileWriter() (io.WriteCloser, error) {
	return &packWriter{s: s}, nil
}

type packWriter struct {
	s   *Store
	buf bytes.Buffer
}

func (w *packWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }

func (w *packWriter) Close() error {
	sum := sha256.Sum256(w.buf.Bytes())
	name := hex.EncodeToString(sum[:16])
	key := object.PackKey(string(w.s.account), string(w.s.repo), name)
	if err := w.s.objects.Put(w.s.ctx(), key, bytes.NewReader(w.buf.Bytes()), int64(w.buf.Len())); err != nil {
		return err
	}
	p, err := packfile.NewParserWithStorage(packfile.NewScanner(bytes.NewReader(w.buf.Bytes())), w.s)
	if err != nil {
		return err
	}
	_, err = p.Parse()
	return err
}
