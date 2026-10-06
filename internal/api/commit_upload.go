package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/mikerudolph/artifacts/internal/api/envelope"
	"github.com/mikerudolph/artifacts/internal/commitupload"
	"github.com/mikerudolph/artifacts/internal/httpstream"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

var errUploadUnauthorized = errors.New("upload authorization expired")

func (s *server) createMultipartCommit(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).EnableFullDuplex()
	select {
	case s.uploadSlots <- struct{}{}:
		defer func() { <-s.uploadSlots }()
	default:
		uploadUnavailable(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Uploads.Timeout)
	defer cancel()
	r = r.WithContext(ctx)
	stream := httpstream.New(w, s.deps.StreamIdle)
	deadline, _ := ctx.Deadline()
	stream.Deadline(deadline)
	defer stream.Close()
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = http.NewResponseController(w).SetReadDeadline(time.Now())
		_ = r.Body.Close()
		close(closed)
	})
	defer func() {
		if !stop() {
			<-closed
		}
	}()
	repo, err := s.routeRepo(r)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	upload, err := s.parseUpload(w, r, stream)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	defer upload.Close()
	stream.Close()
	input := upload.Input
	input.IdempotencyKey = r.Header.Get("Idempotency-Key")
	input.RepositoryCredential, _ = ctx.Value(repositoryCredentialKey{}).(bool)
	input.Authorize = func(ctx context.Context) error { return s.authorizeUpload(r.WithContext(ctx), repo.ID) }
	result, err := s.deps.Repository.Commit(ctx, repo, input)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeOK(w, http.StatusCreated, result)
}

func (s *server) parseUpload(w http.ResponseWriter, r *http.Request, stream *httpstream.Stream) (*commitupload.Upload, error) {
	key := r.Header.Get("Idempotency-Key")
	if key != "" {
		if err := meta.ValidateIdempotencyKey(key); err != nil {
			return nil, err
		}
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" || params["boundary"] == "" || len(params["boundary"]) > 70 {
		return nil, &types.InputError{Message: "expected multipart/form-data with a valid boundary"}
	}
	if r.Header.Get("Content-Encoding") != "" {
		return nil, &types.InputError{Message: "encoded request bodies are not supported"}
	}
	body := http.MaxBytesReader(w, r.Body, s.cfg.Uploads.MaxBytes+commitupload.WireOverhead)
	upload, err := commitupload.Parse(r.Context(), stream.Reader(body), params["boundary"], s.cfg.Uploads.MaxBytes)
	if err == nil {
		return upload, nil
	}
	if r.Context().Err() != nil {
		return nil, r.Context().Err()
	}
	var input *types.InputError
	var path *os.PathError
	var large *http.MaxBytesError
	var timeout net.Error
	switch {
	case errors.As(err, &input), errors.As(err, &path), errors.As(err, &timeout) && timeout.Timeout():
		return nil, err
	case errors.As(err, &large):
		return nil, &types.InputError{Message: "multipart request exceeds wire limit", TooLarge: true}
	default:
		return nil, &types.InputError{Message: "invalid or incomplete multipart body"}
	}
}

func (s *server) authorizeUpload(r *http.Request, id types.RepoID) error {
	repo, err := s.routeRepo(r)
	if err != nil {
		return err
	}
	if repo.ID != id {
		return meta.ErrNotFound
	}
	credential, _ := r.Context().Value(repositoryCredentialKey{}).(bool)
	if !credential {
		if s.controlAuthorized(r) {
			return nil
		}
		return errUploadUnauthorized
	}
	if err := s.deps.RepoAuthorizer.Authorize(r.Context(), repo, bearer(r), true); err != nil {
		if errors.Is(err, types.ErrForbidden) {
			return err
		}
		return errUploadUnauthorized
	}
	if repo.ReadOnly {
		return types.ErrForbidden
	}
	return nil
}

func uploadUnavailable(w http.ResponseWriter) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Retry-After", "1")
	_, body := envelope.Fail(http.StatusServiceUnavailable, envelope.APIError{Code: envelope.CodeInternalError, Kind: "upload_unavailable", Message: "upload capacity unavailable; retry this operation with the same key"})
	envelope.Write(w, http.StatusServiceUnavailable, body)
}

func writeUploadError(w http.ResponseWriter, err error) {
	w.Header().Set("Connection", "close")
	var timeout net.Error
	switch {
	case errors.Is(err, errUploadUnauthorized):
		unauthorized(w)
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		uploadUnavailable(w)
	case errors.As(err, &timeout) && timeout.Timeout():
		_, body := envelope.Fail(http.StatusRequestTimeout, envelope.APIError{Code: envelope.CodeInvalidInput, Kind: "upload_timeout", Message: "upload stalled; retry unchanged content with the same key"})
		envelope.Write(w, http.StatusRequestTimeout, body)
	case errors.Is(err, io.ErrUnexpectedEOF):
		writeErr(w, &types.InputError{Message: "incomplete multipart body"})
	default:
		writeErr(w, err)
	}
}
