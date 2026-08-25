package httpstream

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	reads, writes int
	err           error
}

func (w *deadlineRecorder) SetReadDeadline(time.Time) error {
	w.reads++
	return w.err
}

func (w *deadlineRecorder) SetWriteDeadline(time.Time) error {
	w.writes++
	return w.err
}

func TestProgressRefreshesAndClearsDeadlines(t *testing.T) {
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	stream := New(w, time.Second)
	buf := make([]byte, 3)
	if n, err := stream.Reader(bytes.NewBufferString("abc")).Read(buf); err != nil || n != 3 {
		t.Fatalf("read %d %v", n, err)
	}
	if n, err := stream.Writer(w).Write([]byte("xyz")); err != nil || n != 3 {
		t.Fatalf("write %d %v", n, err)
	}
	stream.Close()
	if w.reads < 3 || w.writes < 3 || w.Body.String() != "xyz" {
		t.Fatalf("deadlines read=%d write=%d body=%q", w.reads, w.writes, w.Body.String())
	}
}

func TestDeadlineErrorsAndUnsupportedWriter(t *testing.T) {
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder(), err: errors.New("deadline failed")}
	stream := New(w, 0)
	if _, err := stream.Reader(bytes.NewBufferString("a")).Read(make([]byte, 1)); err == nil {
		t.Fatal("read deadline error ignored")
	}
	if _, err := stream.Writer(w).Write([]byte("a")); err == nil {
		t.Fatal("write deadline error ignored")
	}
	plain := New(httptest.NewRecorder(), time.Millisecond)
	if _, err := plain.Reader(bytes.NewBufferString("a")).Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
}
