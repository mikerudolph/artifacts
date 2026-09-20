package envelope

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Kind             string  `json:"kind,omitempty"`
	CurrentHead      *string `json:"current_head,omitempty"`
	Code             int     `json:"code"`
	Message          string  `json:"message"`
	DocumentationURL string  `json:"documentation_url,omitempty"`
	Source           *struct {
		Pointer string `json:"pointer,omitempty"`
	} `json:"source,omitempty"`
}

type Envelope[T any] struct {
	Result     T          `json:"result"`
	Success    bool       `json:"success"`
	Errors     []APIError `json:"errors"`
	Messages   []APIError `json:"messages"`
	ResultInfo any        `json:"result_info,omitempty"`
}

func OK[T any](v T) Envelope[T] {
	return Envelope[T]{
		Result:   v,
		Success:  true,
		Errors:   []APIError{},
		Messages: []APIError{},
	}
}

func OKInfo[T any](v T, info any) Envelope[T] {
	env := OK(v)
	env.ResultInfo = info
	return env
}

func Fail(status int, err APIError) (int, Envelope[any]) {
	return status, Envelope[any]{
		Success:  false,
		Errors:   []APIError{err},
		Messages: []APIError{},
	}
}

func Write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
