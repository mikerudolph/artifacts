package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type harness struct {
	cfg    configuration
	base   string
	client *http.Client
}

type envelope[T any] struct {
	Result  T    `json:"result"`
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type responseError struct {
	status    int
	malformed bool
	message   string
}

func (e *responseError) Error() string { return e.message }

func callAPI[T any](ctx context.Context, h harness, method, path string, input any) (T, error) {
	var zero T
	response, data, err := h.request(ctx, method, path, input)
	if err != nil {
		return zero, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return zero, &responseError{status: response.StatusCode, message: "authentication rejected"}
	}
	var wrapped envelope[T]
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return zero, &responseError{status: response.StatusCode, malformed: true, message: "unexpected response shape"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !wrapped.Success {
		message := fmt.Sprintf("REST %s %s failed (%d)", method, path, response.StatusCode)
		if len(wrapped.Errors) > 0 {
			message += ": " + wrapped.Errors[0].Message
		}
		return zero, &responseError{status: response.StatusCode, message: message}
	}
	return wrapped.Result, nil
}

func (h harness) request(ctx context.Context, method, path string, input any) (*http.Response, []byte, error) {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return nil, nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.base+path, body)
	if err != nil {
		return nil, nil, err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.token)
	}
	response, err := h.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		_ = response.Body.Close()
		return nil, nil, readErr
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, data, nil
}

func (h harness) readFile(ctx context.Context, namespace, repo, path string) (string, error) {
	target := repoPath(namespace, repo) + "/file?ref=main&path=" + url.QueryEscape(path)
	response, data, err := h.request(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", &responseError{status: response.StatusCode, message: fmt.Sprintf("read file failed (%d)", response.StatusCode)}
	}
	return string(data), nil
}

func repoPath(namespace, repo string) string {
	return "/namespaces/" + url.PathEscape(namespace) + "/repos/" + url.PathEscape(repo)
}

func repoExists(ctx context.Context, h harness, namespace, repo string) (bool, error) {
	response, _, err := h.request(ctx, http.MethodGet, repoPath(namespace, repo), nil)
	if err != nil {
		return false, err
	}
	_ = response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, &responseError{status: response.StatusCode, message: "repository state could not be read"}
	}
	return true, nil
}

func deleteRepo(ctx context.Context, h harness, namespace, repo string) error {
	_, err := callAPI[map[string]string](ctx, h, http.MethodDelete, repoPath(namespace, repo), nil)
	return err
}
