package commitupload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

type Upload struct {
	Input types.CommitInput
	files []*os.File
}

func (u *Upload) Close() {
	for _, f := range u.files {
		_ = f.Close()
	}
}

func Parse(ctx context.Context, body io.Reader, boundary string, limit int64) (*Upload, error) {
	u := &Upload{}
	err := u.parse(ctx, body, boundary, limit)
	if err != nil {
		u.Close()
		return nil, err
	}
	return u, nil
}

func (u *Upload) parse(ctx context.Context, body io.Reader, boundary string, limit int64) error {
	bounded := &io.LimitedReader{R: contextReader{ctx, body}, N: limit + WireOverhead + 1}
	reader := multipart.NewReader(bounded, boundary)
	part, err := nextPart(reader)
	if err != nil {
		return err
	}
	if part.FormName() != "manifest" {
		return invalid("/manifest", "first part must be manifest")
	}
	media, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return invalid("/manifest", "manifest must have application/json content type")
	}
	manifest, err := readManifest(part, limit)
	if err != nil {
		return err
	}
	u.Input = manifest.input(limit)
	if err := u.readFiles(ctx, reader, manifest, limit); err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, bounded); err != nil {
		return err
	}
	if bounded.N == 0 {
		return tooLarge("/", limit+WireOverhead)
	}
	return nil
}

func nextPart(reader *multipart.Reader) (*multipart.Part, error) {
	part, err := reader.NextRawPart()
	if err != nil {
		return nil, err
	}
	size := 0
	for key, values := range part.Header {
		size += len(key)
		for _, value := range values {
			size += len(value)
		}
	}
	if size > 8192 {
		return nil, tooLarge("/", 8192)
	}
	if part.FormName() == "" {
		return nil, invalid("/", "expected form-data part name")
	}
	if part.Header.Get("Content-Transfer-Encoding") != "" || part.Header.Get("Content-Encoding") != "" || strings.HasPrefix(strings.ToLower(part.Header.Get("Content-Type")), "multipart/") {
		return nil, invalid("/", "encoded or nested file parts are not supported")
	}
	return part, nil
}

func (u *Upload) readFiles(ctx context.Context, reader *multipart.Reader, manifest Manifest, limit int64) error {
	pending := make(map[string]File, len(manifest.Files))
	for _, f := range manifest.Files {
		pending[f.Part] = f
	}
	var total int64
	for {
		part, err := nextPart(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		file, ok := pending[part.FormName()]
		if !ok {
			return invalid("/files", "unexpected or repeated file part")
		}
		delete(pending, part.FormName())
		source, err := u.stage(ctx, part, file, limit-total)
		if err != nil {
			return err
		}
		total += source.Size
		u.Input.Files = append(u.Input.Files, types.CommitFile{Path: file.Path, Mode: file.Mode, Source: source})
	}
	if len(pending) != 0 {
		return invalid("/files", "missing file part")
	}
	return nil
}

func (u *Upload) stage(ctx context.Context, part io.Reader, file File, remaining int64) (*types.CommitSource, error) {
	f, err := os.CreateTemp("", "artifacts-commit-*")
	if err != nil {
		return nil, err
	}
	u.files = append(u.files, f)
	if err := os.Remove(f.Name()); err != nil {
		return nil, err
	}
	hash := sha256.New()
	bound := remaining
	if file.Size != nil && *file.Size < bound {
		bound = *file.Size
	}
	size, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(contextReader{ctx, part}, bound+1))
	if err != nil {
		return nil, err
	}
	field := fmt.Sprintf("/files/%s", file.Part)
	if size > remaining {
		return nil, tooLarge(field, u.Input.ContentLimit)
	}
	if file.Size != nil && size != *file.Size {
		return nil, invalid(field+"/size", "received bytes do not match declared size")
	}
	return &types.CommitSource{Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), Open: func() (io.ReadCloser, error) {
		return io.NopCloser(io.NewSectionReader(f, 0, size)), nil
	}}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
