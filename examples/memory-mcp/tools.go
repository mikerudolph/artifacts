package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newMCPServer(brain *brain) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "artifacts-memory", Version: "0.1.0"}, nil)
	addTool(server, "create_thread", "Create a persistent context for related memories and runs.", brain.createThread)
	addTool(server, "get_thread", "Read a thread with its current memories, runs, and output metadata.", brain.getThread)
	addTool(server, "remember", "Store an intentional fact, note, event, goal, decision, or preference.", brain.remember)
	addTool(server, "recall", "Search current memories, threads, runs, and output metadata using lexical and structured filters.", brain.recall)
	addTool(server, "get_memory", "Read the current form and revision history of one memory.", brain.getMemory)
	addTool(server, "revise_memory", "Supersede a current memory while retaining provenance.", brain.reviseMemory)
	addTool(server, "forget", "Retract a current memory while retaining an auditable tombstone.", brain.forget)
	addTool(server, "start_run", "Start a delegated agent run linked to optional threads.", brain.startRun)
	addTool(server, "finish_run", "Finish a running agent run with a status and summary.", brain.finishRun)
	addTool(server, "store_output", "Store a UTF-8 or base64-encoded native output for a running agent run.", brain.storeOutput)
	addTool(server, "read_output", "Read and integrity-check a stored run output.", brain.readOutput)
	return server
}

func addTool[Input, Output any](server *mcp.Server, name, description string,
	handler func(context.Context, Input) (Output, error),
) {
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description},
		func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Output, error) {
			output, err := handler(ctx, input)
			return nil, output, err
		})
}
