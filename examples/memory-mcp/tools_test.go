package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPProtocolToolsAndCalls(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := newMCPServer(b)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "memory-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertToolSchemas(t, tools.Tools)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_thread",
		Arguments: map[string]any{"title": "Trip planning"}})
	if err != nil || result.IsError {
		t.Fatalf("create thread result=%+v error=%v", result, err)
	}
	var created createThreadOutput
	decodeStructured(t, result.StructuredContent, &created)
	if validateID(created.ThreadID, "thread_") != nil || created.Commit == "" {
		t.Fatalf("created=%+v", created)
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "remember",
		Arguments: map[string]any{"content": "untyped", "kind": "unknown"}})
	if err != nil || !bad.IsError || !strings.Contains(bad.Content[0].(*mcp.TextContent).Text, "invalid kind") {
		t.Fatalf("bad result=%+v error=%v", bad, err)
	}
}

func assertToolSchemas(t *testing.T, tools []*mcp.Tool) {
	t.Helper()
	names := make([]string, 0, len(tools))
	var rememberSchema string
	for _, tool := range tools {
		names = append(names, tool.Name)
		if tool.Name == "remember" {
			data, _ := json.Marshal(tool.InputSchema)
			rememberSchema = string(data)
		}
	}
	sort.Strings(names)
	want := []string{"create_thread", "finish_run", "forget", "get_memory", "get_thread", "read_output",
		"recall", "remember", "revise_memory", "start_run", "store_output"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools=%v", names)
	}
	for _, property := range []string{"starts_at", "ends_at", "due_at", "timezone"} {
		if !strings.Contains(rememberSchema, `"`+property+`"`) {
			t.Fatalf("remember schema lacks %s: %s", property, rememberSchema)
		}
	}
}

func decodeStructured(t *testing.T, value any, target any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil || json.Unmarshal(data, target) != nil {
		t.Fatalf("structured content %v: %v", value, err)
	}
}
