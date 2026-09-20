package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type configuration struct {
	root    string
	account string
	token   string
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}
	cfg, err := loadConfiguration()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	client := newHarness(cfg)
	switch args[0] {
	case "doctor":
		if len(args) != 1 {
			writeUsage(stderr)
			return 2
		}
		result, err := doctor(ctx, client)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "doctor: %s: %v\n", classify(err), err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "healthy account=%s target=%s\n", cfg.account, result)
		return 0
	case "verify-core":
		set := flag.NewFlagSet("verify-core", flag.ContinueOnError)
		set.SetOutput(stderr)
		evidence := set.String("evidence", "", "evidence output directory")
		if err := set.Parse(args[1:]); err != nil || *evidence == "" || set.NArg() != 0 {
			writeUsage(stderr)
			return 2
		}
		report, err := verifyCore(ctx, client, *evidence)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "verify-core: %s: %v\n", report.Classification, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "verified run=%s evidence=%s\n", report.RunID, *evidence)
		return 0
	default:
		writeUsage(stderr)
		return 2
	}
}

func loadConfiguration() (configuration, error) {
	root := strings.TrimRight(env("ARTIFACTS_URL", "http://127.0.0.1:8080"), "/")
	parsed, err := url.Parse(root)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return configuration{}, fmt.Errorf("invalid ARTIFACTS_URL")
	}
	return configuration{root: root, account: env("ARTIFACTS_ACCOUNT", "local"), token: os.Getenv("ARTIFACTS_API_TOKEN")}, nil
}

func newHarness(cfg configuration) harness {
	base := cfg.root + "/client/v4/accounts/" + url.PathEscape(cfg.account) + "/artifacts"
	return harness{cfg: cfg, base: base, client: &http.Client{Timeout: 30 * time.Second}}
}

func writeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: agent-harness doctor | verify-core --evidence DIR")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
