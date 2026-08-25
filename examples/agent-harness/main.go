// Command agent-harness demonstrates REST bootstrap, Git work, and REST readback.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

type harness struct {
	base   string
	bearer string
	client *http.Client
}

type envelope[T any] struct {
	Result  T    `json:"result"`
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func main() {
	root := strings.TrimRight(env("ARTIFACTS_URL", "http://127.0.0.1:8080"), "/")
	account := env("ARTIFACTS_ACCOUNT", "local")
	h := harness{
		base:   root + "/client/v4/accounts/" + url.PathEscape(account) + "/artifacts",
		bearer: os.Getenv("ARTIFACTS_API_TOKEN"), client: &http.Client{Timeout: 30 * time.Second},
	}
	namespace := "agents"
	repoName := fmt.Sprintf("example-session-%d", time.Now().Unix())
	created := mustCall[types.CreateRepoResult](h, http.MethodPost, "/namespaces/"+namespace+"/repos", types.CreateRepoInput{
		Name: types.RepoName(repoName), Description: "example agent session",
	})
	fmt.Println("created", created.Remote)

	commit := mustCall[types.CommitResult](h, http.MethodPost, repoPath(namespace, repoName)+"/commits", types.CommitInput{
		Message: "bootstrap agent", Files: []types.CommitFile{
			{Path: "README.md", Content: "# Agent session\n"},
			{Path: "inputs/task.txt", Content: "Summarize the supplied artifacts.\n"},
		},
	})
	fmt.Println("REST commit", commit.SHA)
	fmt.Printf("initial read: %s", h.readFile(namespace, repoName, "inputs/task.txt"))

	credential := mustCall[types.CreateTokenResult](h, http.MethodPost, "/namespaces/"+namespace+"/credentials", types.CreateTokenInput{
		Repo: types.RepoName(repoName), Scope: types.ScopeWrite, TTL: 3600,
	})
	work, err := os.MkdirTemp("", "artifacts-agent-*")
	must(err)
	defer func() { _ = os.RemoveAll(work) }()
	header := credentialHeader(credential.Plaintext)
	runGit("", "-c", "protocol.version=1", "-c", "http.extraHeader="+header, "clone", created.Remote, work)
	must(os.WriteFile(filepath.Join(work, "result.txt"), []byte("agent finished successfully\n"), 0o600))
	runGit(work, "add", "result.txt")
	runGit(work, "-c", "user.name=Example Agent", "-c", "user.email=agent@example.local", "commit", "-m", "add result")
	runGit(work, "-c", "protocol.version=1", "-c", "http.extraHeader="+header, "push", "origin", "HEAD:main")
	fmt.Printf("Git readback: %s", h.readFile(namespace, repoName, "result.txt"))
}

func mustCall[T any](h harness, method, path string, input any) T {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		must(err)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, h.base+path, body)
	must(err)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+h.bearer)
	}
	response, err := h.client.Do(req)
	must(err)
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	must(err)
	var wrapped envelope[T]
	must(json.Unmarshal(data, &wrapped))
	if !wrapped.Success {
		panic(fmt.Sprintf("REST %s %s failed (%d): %s", method, path, response.StatusCode, data))
	}
	return wrapped.Result
}

func (h harness) readFile(namespace, repo, path string) string {
	target := h.base + repoPath(namespace, repo) + "/file?ref=main&path=" + url.QueryEscape(path)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	must(err)
	if h.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+h.bearer)
	}
	response, err := h.client.Do(req)
	must(err)
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	must(err)
	if response.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("read file failed (%d): %s", response.StatusCode, data))
	}
	return string(data)
}

func repoPath(namespace, repo string) string {
	return "/namespaces/" + url.PathEscape(namespace) + "/repos/" + url.PathEscape(repo)
}

func credentialHeader(credential string) string {
	secret, _, _ := strings.Cut(credential, "?")
	return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("agent:"+secret))
}

func runGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	must(cmd.Run())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
