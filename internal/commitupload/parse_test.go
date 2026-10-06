package commitupload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/types"
)

type testPart struct{ name, content string }

func requestBody(t *testing.T, manifest string, parts ...testPart) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	w := multipart.NewWriter(body)
	p, err := w.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="manifest"`}, "Content-Type": {"application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(p, manifest); err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		p, err := w.CreateFormFile(part.name, "ignored")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(p, part.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.Boundary()
}

func TestBinaryUnknownSizeAndCleanup(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	payload := string([]byte{0, 255, 128, 13, 10})
	body, boundary := requestBody(t, `{"files":[{"path":"bin","part":"b","mode":"100755"},{"path":"empty","part":"e","size":0}]}`, testPart{"e", ""}, testPart{"b", payload})
	upload, err := Parse(t.Context(), body, boundary, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer upload.Close()
	source := upload.Input.Files[1].Source
	expected := sha256.Sum256([]byte(payload))
	if source.Size != 5 || source.SHA256 != hex.EncodeToString(expected[:]) || upload.Input.Files[1].Mode != "100755" {
		t.Fatal("source metadata changed")
	}
	for range 2 {
		r, err := source.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil || string(got) != payload {
			t.Fatalf("content: %x %v", got, err)
		}
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("named scratch files survive: %v %v", entries, err)
	}
	upload.Close()
	r, _ := source.Open()
	if _, err := io.ReadAll(r); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("staged handle survived cleanup: %v", err)
	}
}

func TestRejectManifestAndParts(t *testing.T) {
	cases := []struct {
		name, manifest string
		parts          []testPart
		large          bool
	}{
		{"unknown", `{"files":[],"oops":true}`, nil, false},
		{"duplicate member", `{"files":[],"files":[]}`, nil, false},
		{"nested duplicate", `{"author":{"name":"a","name":"b"}}`, nil, false},
		{"trailing", `{} {}`, nil, false},
		{"syntax", `{`, nil, false},
		{"oversized manifest", strings.Repeat(" ", ManifestLimit+1), nil, true},
		{"part identifier", `{"files":[{"path":"a","part":"manifest"}]}`, nil, false},
		{"duplicate part", `{"files":[{"part":"a"},{"part":"a"}]}`, nil, false},
		{"bad mode", `{"files":[{"part":"a","mode":"120000"}]}`, nil, false},
		{"negative", `{"files":[{"part":"a","size":-1}]}`, nil, false},
		{"declared excess", `{"files":[{"part":"a","size":6}]}`, nil, true},
		{"actual excess", `{"files":[{"part":"a"}]}`, []testPart{{"a", "123456"}}, true},
		{"short", `{"files":[{"part":"a","size":5}]}`, []testPart{{"a", "1234"}}, false},
		{"long", `{"files":[{"part":"a","size":3}]}`, []testPart{{"a", "1234"}}, false},
		{"missing", `{"files":[{"part":"a"}]}`, nil, false},
		{"extra", `{"files":[]}`, []testPart{{"a", "x"}}, false},
		{"repeated", `{"files":[{"part":"a"}]}`, []testPart{{"a", "x"}, {"a", "x"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, boundary := requestBody(t, tc.manifest, tc.parts...)
			upload, err := Parse(t.Context(), body, boundary, 5)
			var input *types.InputError
			if upload != nil || !errors.As(err, &input) || input.TooLarge != tc.large {
				t.Fatalf("upload=%v error=%v", upload, err)
			}
		})
	}
}

func TestCancellationTruncationAndWireBound(t *testing.T) {
	body, boundary := requestBody(t, `{"replace":true}`)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Parse(ctx, body, boundary, 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	truncated := body.String()[:body.Len()-5]
	if _, err := Parse(t.Context(), strings.NewReader(truncated), boundary, 5); err == nil {
		t.Fatal("accepted incomplete body")
	}
	full := body.String() + strings.Repeat("x", WireOverhead)
	if _, err := Parse(t.Context(), strings.NewReader(full), boundary, 5); err == nil {
		t.Fatal("accepted excessive epilogue")
	}
}
