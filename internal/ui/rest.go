package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mikerudolph/artifacts/internal/types"
)

const maxRESTResponseBytes = 4 << 20

var errResponseTooLarge = errors.New("REST response exceeds preview limit")

type restClient struct {
	handler http.Handler
}

type fileRequest struct {
	account   string
	namespace string
	repo      string
	ref       string
	path      string
}

type treeResponse struct {
	Ref     string            `json:"ref"`
	Commit  string            `json:"commit"`
	Tree    string            `json:"tree"`
	Path    string            `json:"path"`
	Entries []types.TreeEntry `json:"entries"`
}

type responseError struct {
	status  int
	message string
}

func (e responseError) Error() string {
	if e.message != "" {
		return e.message
	}
	return fmt.Sprintf("REST API returned HTTP %d", e.status)
}

func isRESTStatus(err error, status int) bool {
	var response responseError
	return errors.As(err, &response) && response.status == status
}

type responseBuffer struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	limit    int64
	overflow bool
}

func newResponseBuffer(limit int64) *responseBuffer {
	return &responseBuffer{header: make(http.Header), limit: limit}
}

func (r *responseBuffer) Header() http.Header { return r.header }

func (r *responseBuffer) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *responseBuffer) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	remaining := r.limit - int64(r.body.Len())
	if remaining <= 0 {
		r.overflow = true
		return 0, errResponseTooLarge
	}
	if int64(len(body)) > remaining {
		n, _ := r.body.Write(body[:remaining])
		r.overflow = true
		return n, errResponseTooLarge
	}
	return r.body.Write(body)
}

func (c restClient) request(ctx context.Context, path string, limit int64) (*responseBuffer, error) {
	requestContext := context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	response := newResponseBuffer(limit)
	c.handler.ServeHTTP(response, req)
	if response.status == 0 {
		response.status = http.StatusOK
	}
	return response, nil
}

func (c restClient) getJSON(ctx context.Context, path string, result any) error {
	response, err := c.request(ctx, path, maxRESTResponseBytes)
	if err != nil {
		return err
	}
	if response.overflow {
		return errResponseTooLarge
	}
	var envelope struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(response.body.Bytes(), &envelope); err != nil {
		if response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
			return responseError{status: response.status}
		}
		return fmt.Errorf("decode REST response: %w", err)
	}
	if response.status < http.StatusOK || response.status >= http.StatusMultipleChoices || !envelope.Success {
		message := ""
		if len(envelope.Errors) != 0 {
			message = envelope.Errors[0].Message
		}
		return responseError{status: response.status, message: message}
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode REST result: %w", err)
	}
	return nil
}

func (c restClient) namespaces(ctx context.Context, account string) ([]types.Namespace, string) {
	var result []types.Namespace
	err := c.getJSON(ctx, accountAPIPath(account)+"/namespaces?limit=200", &result)
	return result, errorText(err)
}

func (c restClient) repos(ctx context.Context, account, namespace string) ([]types.Repo, string) {
	var result []types.Repo
	err := c.getJSON(ctx, namespaceAPIPath(account, namespace)+"/repos?limit=200", &result)
	return result, errorText(err)
}

func (c restClient) repo(ctx context.Context, account, namespace, repo string) (types.Repo, error) {
	var result types.Repo
	err := c.getJSON(ctx, repoAPIPath(account, namespace, repo), &result)
	return result, err
}

func (c restClient) refs(ctx context.Context, account, namespace, repo string) ([]types.Ref, error) {
	var result []types.Ref
	err := c.getJSON(ctx, repoAPIPath(account, namespace, repo)+"/refs", &result)
	return result, err
}

func (c restClient) tree(ctx context.Context, account, namespace, repo, ref, path string) (treeResponse, error) {
	var result treeResponse
	query := url.Values{"ref": {ref}, "path": {path}}
	err := c.getJSON(ctx, repoAPIPath(account, namespace, repo)+"/tree?"+query.Encode(), &result)
	return result, err
}

func (c restClient) log(ctx context.Context, account, namespace, repo, ref string) ([]types.LogEntry, error) {
	var result []types.LogEntry
	query := url.Values{"ref": {ref}, "limit": {"50"}}
	err := c.getJSON(ctx, repoAPIPath(account, namespace, repo)+"/log?"+query.Encode(), &result)
	return result, err
}

func (c restClient) wal(ctx context.Context, account, namespace, repo string) ([]types.PackWAL, error) {
	var result []types.PackWAL
	err := c.getJSON(ctx, repoAPIPath(account, namespace, repo)+"/wal", &result)
	return result, err
}

func (c restClient) file(ctx context.Context, account, namespace, repo, ref, path string) ([]byte, error) {
	response, err := c.request(ctx, fileAPIPath(account, namespace, repo, ref, path), maxPreviewBytes)
	if err != nil {
		return nil, err
	}
	if response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
		return nil, decodeResponseError(response)
	}
	if response.overflow {
		return nil, errResponseTooLarge
	}
	return response.body.Bytes(), nil
}

func (c restClient) serveFile(w http.ResponseWriter, ctx context.Context, file fileRequest) {
	path := fileAPIPath(file.account, file.namespace, file.repo, file.ref, file.path)
	requestContext := context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, path, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c.handler.ServeHTTP(w, req)
}

func decodeResponseError(response *responseBuffer) error {
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(response.body.Bytes(), &envelope)
	message := ""
	if len(envelope.Errors) != 0 {
		message = envelope.Errors[0].Message
	}
	return responseError{status: response.status, message: message}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func accountAPIPath(account string) string {
	return "/client/v4/accounts/" + url.PathEscape(account) + "/artifacts"
}

func namespaceAPIPath(account, namespace string) string {
	return accountAPIPath(account) + "/namespaces/" + url.PathEscape(namespace)
}

func repoAPIPath(account, namespace, repo string) string {
	return namespaceAPIPath(account, namespace) + "/repos/" + url.PathEscape(repo)
}

func fileAPIPath(account, namespace, repo, ref, path string) string {
	query := url.Values{"ref": {ref}, "path": {path}}
	return repoAPIPath(account, namespace, repo) + "/file?" + query.Encode()
}

func browserRepoPath(account, namespace, repo string) string {
	return "/" + url.PathEscape(account) + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(repo)
}

func escapeRepositoryPath(path string) string {
	parts := strings.Split(path, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func withRef(path, ref string) string {
	if ref == "" {
		return path
	}
	return path + "?" + url.Values{"ref": {ref}}.Encode()
}
