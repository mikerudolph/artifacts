package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type configuration struct {
	Root      string
	Account   string
	Token     string
	Namespace string
	Repo      string
}

type repositoryAccess struct {
	Remote     string
	Credential string
}

type repositoryResult struct {
	Remote string `json:"remote"`
	Token  string `json:"token"`
}

type credentialResult struct {
	Plaintext string `json:"plaintext"`
}

type artifactsError struct {
	Status    int
	Malformed bool
	Operation string
}

func (e *artifactsError) Error() string {
	if e.Malformed {
		return "Artifacts returned a malformed response"
	}
	if e.Status == 0 {
		return "Artifacts is unreachable"
	}
	return fmt.Sprintf("Artifacts %s failed with status %d", e.Operation, e.Status)
}

type artifactsClient struct {
	base   string
	token  string
	client *http.Client
}

func loadConfiguration() (configuration, error) {
	cfg := configuration{
		Root:      env("ARTIFACTS_URL", "http://127.0.0.1:8080"),
		Account:   env("ARTIFACTS_ACCOUNT", "local"),
		Token:     env("ARTIFACTS_API_TOKEN", ""),
		Namespace: env("ARTIFACTS_MEMORY_NAMESPACE", "memory"),
		Repo:      env("ARTIFACTS_MEMORY_REPO", "brain"),
	}
	parsed, err := url.Parse(cfg.Root)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return configuration{}, errors.New("ARTIFACTS_URL must be an absolute URL")
	}
	if cfg.Account == "" || cfg.Namespace == "" || cfg.Repo == "" {
		return configuration{}, errors.New("artifacts account, namespace, and repository must be non-empty")
	}
	cfg.Root = strings.TrimRight(cfg.Root, "/")
	return cfg, nil
}

func newArtifactsClient(cfg configuration) *artifactsClient {
	base := cfg.Root + "/client/v4/accounts/" + url.PathEscape(cfg.Account) + "/artifacts"
	return &artifactsClient{base: base, token: cfg.Token, client: &http.Client{Timeout: 30 * time.Second}}
}

func (c *artifactsClient) ensureRepository(ctx context.Context, namespace, repo string) (repositoryAccess, error) {
	path := repoAPIPath(namespace, repo)
	var current repositoryResult
	err := c.call(ctx, http.MethodGet, path, nil, &current)
	if err == nil {
		return c.issueCredential(ctx, namespace, repo, current.Remote)
	}
	var apiErr *artifactsError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		return repositoryAccess{}, err
	}
	input := map[string]any{"name": repo, "description": "Personal memory MCP brain"}
	if err := c.call(ctx, http.MethodPost, "/namespaces/"+url.PathEscape(namespace)+"/repos", input, &current); err != nil {
		if !isConflict(err) {
			return repositoryAccess{}, err
		}
		if err := c.call(ctx, http.MethodGet, path, nil, &current); err != nil {
			return repositoryAccess{}, err
		}
		return c.issueCredential(ctx, namespace, repo, current.Remote)
	}
	if current.Remote == "" || current.Token == "" {
		return repositoryAccess{}, &artifactsError{Malformed: true}
	}
	return repositoryAccess{Remote: current.Remote, Credential: current.Token}, nil
}

func isConflict(err error) bool {
	var apiErr *artifactsError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict
}

func (c *artifactsClient) issueCredential(ctx context.Context, namespace, repo, remote string) (repositoryAccess, error) {
	input := map[string]any{"repo": repo, "scope": "write", "ttl": 3600}
	var result credentialResult
	path := "/namespaces/" + url.PathEscape(namespace) + "/credentials"
	if err := c.call(ctx, http.MethodPost, path, input, &result); err != nil {
		return repositoryAccess{}, err
	}
	if remote == "" || result.Plaintext == "" {
		return repositoryAccess{}, &artifactsError{Malformed: true}
	}
	return repositoryAccess{Remote: remote, Credential: result.Plaintext}, nil
}

func (c *artifactsClient) call(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return errors.New("encode Artifacts request")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return errors.New("create Artifacts request")
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return &artifactsError{Operation: strings.ToLower(method)}
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return &artifactsError{Status: response.StatusCode, Malformed: true}
	}
	var envelope json.RawMessage
	var header struct {
		Result  json.RawMessage `json:"result"`
		Success bool            `json:"success"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return &artifactsError{Status: response.StatusCode, Malformed: true}
	}
	envelope = header.Result
	if response.StatusCode < 200 || response.StatusCode >= 300 || !header.Success {
		return &artifactsError{Status: response.StatusCode, Operation: strings.ToLower(method)}
	}
	if output != nil && (len(envelope) == 0 || json.Unmarshal(envelope, output) != nil) {
		return &artifactsError{Status: response.StatusCode, Malformed: true}
	}
	return nil
}

func repoAPIPath(namespace, repo string) string {
	return "/namespaces/" + url.PathEscape(namespace) + "/repos/" + url.PathEscape(repo)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
