package envelope

import (
	"errors"
	"net/http"

	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// FromDomain maps a domain error to an HTTP status and API error.
func FromDomain(err error) (int, APIError) {
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
		return http.StatusNotFound, APIError{Code: CodeNotFound, Message: "File not found"}
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
