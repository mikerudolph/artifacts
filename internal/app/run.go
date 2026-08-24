package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
)

// Run executes a CLI command. Returns process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		WriteUsage(stdout)
		return 0
	}
	switch args[0] {
	case "serve":
		return runServe(ctx, stderr)
	case "migrate":
		return runMigrate(stderr)
	case "token":
		return runToken(stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		return 2
	}
}

func runServe(ctx context.Context, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	h, err := Handler(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stderr, "listening on %s\n", cfg.HTTP.Addr)
	srv := &http.Server{Addr: cfg.HTTP.Addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runMigrate(stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := postgres.Migrate(cfg.Postgres.DSN); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runToken(stdout, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if cfg.Auth.APIToken == "" {
		_, _ = fmt.Fprintln(stderr, "set ARTIFACTS_API_TOKEN")
		return 1
	}
	_, _ = fmt.Fprintln(stdout, cfg.Auth.APIToken)
	return 0
}
