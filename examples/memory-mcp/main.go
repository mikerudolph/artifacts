package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		writeDiagnostic(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfiguration()
	if err != nil {
		return err
	}
	access, err := newArtifactsClient(cfg).ensureRepository(ctx, cfg.Namespace, cfg.Repo)
	if err != nil {
		return err
	}
	workspace, err := cloneWorkspace(ctx, access.Remote, access.Credential)
	if err != nil {
		return err
	}
	defer func() { _ = workspace.Close() }()
	return newMCPServer(newBrain(workspace)).Run(ctx, &mcp.StdioTransport{})
}

func writeDiagnostic(w io.Writer, err error) {
	if err != nil {
		_, _ = fmt.Fprintf(w, "memory-mcp: %v\n", err)
	}
}
