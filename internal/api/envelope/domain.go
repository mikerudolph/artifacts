package envelope

import (
	"errors"
	"net/http"

	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func FromDomain(err error) (int, APIError) {
	if status, out, ok := detailedError(err); ok {
		return status, out
	}
	switch {
	case err == nil:
		return http.StatusInternalServerError, APIError{Code: CodeInternalError, Message: "internal error"}
	case errors.Is(err, types.ErrInvalidName):
		return http.StatusBadRequest, APIError{Code: CodeInvalidRepoName, Message: err.Error()}
	case errors.Is(err, types.ErrInvalidTTL):
		return http.StatusBadRequest, APIError{Code: CodeInvalidTTL, Message: err.Error()}
	case errors.Is(err, types.ErrInvalidScope),
		errors.Is(err, types.ErrInvalidState),
		errors.Is(err, types.ErrInvalidSort),
		errors.Is(err, types.ErrInvalidStatus),
		errors.Is(err, types.ErrInvalidJurisdiction):
		return http.StatusBadRequest, APIError{Code: CodeInvalidInput, Message: err.Error()}
	case errors.Is(err, meta.ErrNotFound):
		return http.StatusNotFound, APIError{Code: CodeNotFound, Kind: "not_found", Message: "Resource not found"}
	case errors.Is(err, meta.ErrAlreadyExists):
		return http.StatusConflict, APIError{Code: CodeAlreadyExists, Message: err.Error()}
	case errors.Is(err, jobs.ErrInvalidURL):
		return http.StatusBadRequest, APIError{Code: CodeInvalidURL, Message: err.Error()}
	case errors.Is(err, jobs.ErrRemoteAuth):
		return http.StatusBadRequest, APIError{Code: CodeRemoteAuthRequired, Message: err.Error()}
	case errors.Is(err, jobs.ErrUpstream):
		return http.StatusBadGateway, APIError{Code: CodeUpstreamUnavailable, Message: err.Error()}
	case errors.Is(err, jobs.ErrBusy):
		return http.StatusConflict, APIError{Code: CodeImportInProgress, Message: err.Error()}
	default:
		return http.StatusInternalServerError, APIError{Code: CodeInternalError, Message: "internal error"}
	}
}

func detailedError(err error) (int, APIError, bool) {
	var input *types.InputError
	if errors.As(err, &input) {
		status := http.StatusBadRequest
		kind := "invalid_input"
		if input.TooLarge {
			status, kind = http.StatusRequestEntityTooLarge, "payload_too_large"
		}
		out := APIError{Code: CodeInvalidInput, Kind: kind, Message: input.Message}
		out.Source = &struct {
			Pointer string `json:"pointer,omitempty"`
		}{Pointer: input.Field}
		return status, out, true
	}
	var conflict *types.HeadConflict
	if errors.As(err, &conflict) {
		return http.StatusConflict, APIError{Code: CodePublicationConflict, Kind: "head_conflict", Message: conflict.Error(), CurrentHead: &conflict.Current}, true
	}
	switch {
	case errors.Is(err, types.ErrForbidden):
		return http.StatusForbidden, APIError{Code: CodeForbidden, Kind: "forbidden", Message: err.Error()}, true
	case errors.Is(err, types.ErrIdempotencyConflict):
		return http.StatusConflict, APIError{Code: CodeIdempotencyConflict, Kind: "idempotency_conflict", Message: err.Error()}, true
	case errors.Is(err, meta.ErrCASConflict):
		return http.StatusConflict, APIError{Code: CodePublicationConflict, Kind: "publication_conflict", Message: "publication state changed; read the current head before retrying"}, true
	}
	return 0, APIError{}, false
}
