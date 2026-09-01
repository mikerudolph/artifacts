package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/app"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestArtifactsMCPIntegration(t *testing.T) {
	testkit.GitAvailable(t)
	dsn := testkit.Postgres(t)
	server := httptest.NewUnstartedServer(nil)
	publicURL := "http://" + server.Listener.Addr().String()
	cfg := config.Config{
		HTTP:     config.HTTP{Addr: ":0", PublicURL: publicURL},
		Auth:     config.Auth{Mode: "token", APIToken: "control-secret"},
		Storage:  config.Storage{Backend: "fs", FS: config.FS{Path: t.TempDir()}},
		Cache:    config.Cache{Path: filepath.Join(t.TempDir(), "cache")},
		Postgres: config.Postgres{DSN: dsn}, Account: config.Account{DefaultID: "local"},
	}
	handler, err := app.Handler(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	clientCfg := configuration{Root: server.URL, Account: "local", Token: "control-secret", Namespace: "memory", Repo: "brain"}
	access, err := newArtifactsClient(clientCfg).ensureRepository(context.Background(), "memory", "brain")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := cloneWorkspace(context.Background(), access.Remote, access.Credential)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	exerciseTwoMCPSessions(t, newMCPServer(newBrain(workspace)))
	assertReconstructedBrain(t, clientCfg)
	assertCommandServer(t, clientCfg)
}

func exerciseTwoMCPSessions(t *testing.T, server *mcp.Server) {
	t.Helper()
	one := connectTestMCP(t, server)
	defer func() { _ = one.Close() }()
	two := connectTestMCP(t, server)
	defer func() { _ = two.Close() }()
	thread := callToolAs[createThreadOutput](t, one, "create_thread", map[string]any{"title": "Raise proposal"})
	memory := callToolAs[rememberOutput](t, one, "remember", map[string]any{
		"content": "Meeting Alex Tuesday", "kind": "event", "thread_ids": []string{thread.ThreadID},
		"starts_at": "2026-09-01T14:00:00-03:00", "timezone": "America/Halifax",
	})
	run := callToolAs[startRunOutput](t, two, "start_run", map[string]any{
		"agent": "proposal-agent", "purpose": "Draft the raise proposal", "thread_ids": []string{thread.ThreadID},
	})
	raw := []byte{0, 1, 2, 0xff}
	output := callToolAs[storeOutputOutput](t, two, "store_output", map[string]any{
		"run_id": run.RunID, "name": "proposal.bin", "content": base64.StdEncoding.EncodeToString(raw),
		"encoding": "base64", "media_type": "application/octet-stream",
	})
	callToolAs[finishRunOutput](t, two, "finish_run", map[string]any{
		"run_id": run.RunID, "status": "completed", "summary": "Proposal drafted",
	})
	read := callToolAs[readOutputOutput](t, one, "read_output", map[string]any{"run_id": run.RunID, "output_id": output.OutputID})
	if read.Content != base64.StdEncoding.EncodeToString(raw) || read.Encoding != "base64" {
		t.Fatalf("native output readback=%+v", read)
	}
	revision := callToolAs[reviseMemoryOutput](t, two, "revise_memory", map[string]any{
		"memory_id": memory.MemoryID, "content": "Meeting Alex Tuesday about launch", "reason": "agenda added",
	})
	callToolAs[forgetOutput](t, one, "forget", map[string]any{"memory_id": revision.ReplacementMemoryID, "reason": "cancelled"})
	history := callToolAs[memoryOutput](t, two, "get_memory", map[string]any{"memory_id": memory.MemoryID})
	if len(history.History) != 3 || history.Current.Metadata.Operation != "retract" {
		t.Fatalf("history=%+v", history)
	}
}

func connectTestMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func callToolAs[Output any](t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) Output {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("call %s: %+v %v", name, result, err)
	}
	var output Output
	decodeStructured(t, result.StructuredContent, &output)
	return output
}

func assertReconstructedBrain(t *testing.T, cfg configuration) {
	t.Helper()
	access, err := newArtifactsClient(cfg).ensureRepository(context.Background(), cfg.Namespace, cfg.Repo)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := cloneWorkspace(context.Background(), access.Remote, access.Credential)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	result, err := newBrain(workspace).recall(context.Background(), recallInput{Query: "proposal", Types: []string{"thread", "run", "output"}})
	if err != nil || len(result.Matches) < 2 {
		t.Fatalf("reconstructed recall=%+v %v", result, err)
	}
}

func assertCommandServer(t *testing.T, cfg configuration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", ".")
	command.Env = append(os.Environ(), "ARTIFACTS_URL="+cfg.Root, "ARTIFACTS_ACCOUNT="+cfg.Account,
		"ARTIFACTS_API_TOKEN="+cfg.Token, "ARTIFACTS_MEMORY_NAMESPACE="+cfg.Namespace, "ARTIFACTS_MEMORY_REPO="+cfg.Repo)
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "command-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "recall", Arguments: map[string]any{
		"query": "proposal", "types": []string{"thread", "run", "output"},
	}})
	if err != nil || result.IsError {
		t.Fatalf("command recall=%+v error=%v stderr=%s", result, err, stderr)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("command close: %v stderr=%s", err, stderr)
	}
	if strings.Contains(stderr.String(), cfg.Token) || strings.Contains(stderr.String(), "Raise proposal") {
		t.Fatalf("command stderr leaked protected data: %s", stderr)
	}
}
