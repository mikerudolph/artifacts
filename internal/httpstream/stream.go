package httpstream

import (
	"errors"
	"io"
	"net/http"
	"time"
)

const DefaultIdleTimeout = 30 * time.Second

type Stream struct {
	controller *http.ResponseController
	idle       time.Duration
}

func New(w http.ResponseWriter, idle time.Duration) *Stream {
	if idle <= 0 {
		idle = DefaultIdleTimeout
	}
	return &Stream{controller: http.NewResponseController(w), idle: idle}
}

func (s *Stream) Reader(r io.Reader) io.Reader {
	return &progressReader{reader: r, stream: s}
}

func (s *Stream) Writer(w http.ResponseWriter) http.ResponseWriter {
	return &progressWriter{ResponseWriter: w, stream: s}
}

func (s *Stream) Close() {
	_ = s.controller.SetReadDeadline(time.Time{})
	_ = s.controller.SetWriteDeadline(time.Time{})
}

func (s *Stream) setRead() error {
	return ignoreUnsupported(s.controller.SetReadDeadline(time.Now().Add(s.idle)))
}

func (s *Stream) setWrite() error {
	return ignoreUnsupported(s.controller.SetWriteDeadline(time.Now().Add(s.idle)))
}

func ignoreUnsupported(err error) error {
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

type progressReader struct {
	reader io.Reader
	stream *Stream
}

func (r *progressReader) Read(p []byte) (int, error) {
	if err := r.stream.setRead(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		_ = r.stream.setRead()
	}
	return n, err
}

type progressWriter struct {
	http.ResponseWriter
	stream *Stream
}

func (w *progressWriter) Write(p []byte) (int, error) {
	if err := w.stream.setWrite(); err != nil {
		return 0, err
	}
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		_ = w.stream.setWrite()
	}
	return n, err
}

func (w *progressWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
