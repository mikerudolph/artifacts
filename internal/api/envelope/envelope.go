package envelope

import (
	"encoding/json"
	"net/http"
)

// APIError is one Cloudflare v4 error or message entry.
type APIError struct {
	Code             int    `json:"code"`
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url,omitempty"`
	Source           *struct {
		Pointer string `json:"pointer,omitempty"`
	} `json:"source,omitempty"`
}

// Envelope is the Cloudflare v4 JSON wrapper.
type Envelope[T any] struct {
	Result     T          `json:"result"`
	Success    bool       `json:"success"`
	Errors     []APIError `json:"errors"`
	Messages   []APIError `json:"messages"`
	ResultInfo any        `json:"result_info,omitempty"`
}

// OK wraps a successful result.
func OK[T any](v T) Envelope[T] {
	return Envelope[T]{
		Result:   v,
		Success:  true,
		Errors:   []APIError{},
		Messages: []APIError{},
	}
}

// OKInfo wraps a successful result with pagination metadata.
func OKInfo[T any](v T, info any) Envelope[T] {
	env := OK(v)
	env.ResultInfo = info
	return env
}

// Fail wraps a single API error. Result is null.
func Fail(status int, err APIError) (int, Envelope[any]) {
	return status, Envelope[any]{
		Success:  false,
		Errors:   []APIError{err},
		Messages: []APIError{},
	}
}

// Write writes JSON with the given HTTP status.
func Write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
