package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"

	"github.com/mikerudolph/artifacts/internal/commitupload"
	"github.com/mikerudolph/artifacts/internal/types"
)

const binaryText = "# Binary verification\n"
const binaryScript = "echo binary-verification\n"

type binaryFixture struct {
	path, digest, head string
	size               int64
	result             types.CommitResult
}

func (f binaryFixture) manifest() commitupload.Manifest {
	return commitupload.Manifest{Message: "binary verification", ExpectedHead: &f.head, Files: []commitupload.File{
		{Path: "binary.bin", Part: "binary", Size: &f.size},
		{Path: "binary.md", Part: "text"},
		{Path: "binary.sh", Part: "script", Mode: "100755"},
	}}
}

func uploadBinary(ctx context.Context, h harness, state *driveState, f binaryFixture, key string, manifest commitupload.Manifest) (types.CommitResult, int, error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	done := make(chan error, 1)
	go func() {
		err := writeBinaryBody(multipartWriter, manifest, f.path)
		if err == nil {
			err = multipartWriter.Close()
		}
		_ = writer.CloseWithError(err)
		done <- err
	}()
	defer func() { _ = reader.Close(); <-done }()
	req, err := http.NewRequestWithContext(ctx, "POST", h.base+repoPath(state.namespace, state.repo)+"/commits", reader)
	if err != nil {
		return types.CommitResult{}, 0, err
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+state.credential)
	req.Header.Set("Idempotency-Key", key)
	response, err := h.client.Do(req)
	if err != nil {
		return types.CommitResult{}, 0, err
	}
	defer func() { _ = response.Body.Close() }()
	var body envelope[types.CommitResult]
	err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body)
	if err != nil {
		return body.Result, response.StatusCode, err
	}
	if response.StatusCode == 201 && !body.Success {
		return body.Result, response.StatusCode, fmt.Errorf("unsuccessful commit envelope")
	}
	return body.Result, response.StatusCode, nil
}

func writeBinaryBody(w *multipart.Writer, manifest commitupload.Manifest, path string) error {
	part, err := w.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="manifest"`}, "Content-Type": {"application/json"}})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(part).Encode(manifest); err != nil {
		return err
	}
	for _, entry := range []struct{ name, text string }{{"text", binaryText}, {"script", binaryScript}} {
		part, err := w.CreateFormFile(entry.name, entry.name)
		if err != nil {
			return err
		}
		if _, err := io.Copy(part, strings.NewReader(entry.text)); err != nil {
			return err
		}
	}
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	part, err = w.CreateFormFile("binary", "content.bin")
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}
