package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/repository"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/types"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		WriteUsage(stdout)
		return 0
	}
	switch args[0] {
	case "serve":
		return runServe(ctx, stderr)
	case "dev":
		return runDev(ctx, args[1:], stderr)
	case "compact":
		return runCompact(ctx, args[1:], stderr)
	case "bootstrap":
		return runBootstrap(ctx, args[1:], stdout, stderr)
	case "migrate":
		return runMigrate(stderr)
	case "token":
		return runToken(ctx, args[1:], stdout, stderr)
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
	return listen(ctx, cfg.HTTP.Addr, h, stderr)
}

func listen(ctx context.Context, addr string, h http.Handler, stderr io.Writer) int {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		closeHandler(h)
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return serveListener(ctx, listener, h, stderr)
}

func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
}

func runDev(ctx context.Context, args []string, stderr io.Writer) int {
	set := flag.NewFlagSet("dev", flag.ContinueOnError)
	set.SetOutput(stderr)
	addr := set.String("addr", "127.0.0.1:8080", "listen address")
	if err := set.Parse(args); err != nil {
		return 2
	}
	if !loopbackAddress(*addr) {
		_, _ = fmt.Fprintln(stderr, "dev server requires a loopback address")
		return 2
	}
	cfg, err := config.LoadNoAuth()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	cfg.HTTP.Addr = *addr
	cfg.HTTP.PublicURL = "http://" + *addr
	h, err := buildHandler(ctx, cfg, true)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return listenDev(ctx, cfg.HTTP.Addr, h, stderr)
}

func listenDev(ctx context.Context, addr string, h http.Handler, stderr io.Writer) int {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		closeHandler(h)
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() { _ = listener.Close() }()
	tcp, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !tcp.IP.IsLoopback() {
		closeHandler(h)
		_, _ = fmt.Fprintln(stderr, "development listener did not resolve to loopback")
		return 2
	}
	return serveListener(ctx, listener, h, stderr)
}

func loopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func runMigrate(stderr io.Writer) int {
	cfg, err := config.LoadDatabase()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := postgres.Migrate(cfg.DSN); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runToken(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		cfg, err := config.Load()
		if err != nil || cfg.Auth.APIToken == "" {
			_, _ = fmt.Fprintln(stderr, "usage: artifacts token create --account ACCOUNT")
			return 1
		}
		_, _ = fmt.Fprintln(stdout, cfg.Auth.APIToken)
		return 0
	}
	if args[0] != "create" {
		_, _ = fmt.Fprintln(stderr, "usage: artifacts token create --account ACCOUNT")
		return 2
	}
	set := flag.NewFlagSet("token create", flag.ContinueOnError)
	set.SetOutput(stderr)
	account := set.String("account", "local", "tenant account")
	if err := set.Parse(args[1:]); err != nil {
		return 2
	}
	cfg, err := config.LoadNoAuth()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if !cfg.Postgres.SkipMigrations {
		if err := postgres.Migrate(cfg.Postgres.DSN); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	metadata, err := postgres.Open(ctx, cfg.Postgres.DSN)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeMetadata(metadata)
	acct := types.AccountID(*account)
	if err := metadata.Accounts().Ensure(ctx, acct); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	plain, hash, err := auth.MintAPI()
	if err == nil {
		_, err = metadata.APITokens().Create(ctx, types.APIToken{AccountID: acct, Hash: hash})
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, plain)
	return 0
}

func runCompact(ctx context.Context, args []string, stderr io.Writer) int {
	set := flag.NewFlagSet("compact", flag.ContinueOnError)
	set.SetOutput(stderr)
	account := set.String("account", "local", "tenant account")
	namespace := set.String("namespace", "default", "namespace")
	name := set.String("repo", "", "repository")
	if err := set.Parse(args); err != nil || *name == "" {
		_, _ = fmt.Fprintln(stderr, "usage: artifacts compact --account A --namespace N --repo R")
		return 2
	}
	cfg, err := config.LoadNoAuth()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if !cfg.Postgres.SkipMigrations {
		if err := postgres.Migrate(cfg.Postgres.DSN); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	metadata, err := postgres.Open(ctx, cfg.Postgres.DSN)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeMetadata(metadata)
	objects, err := openObjects(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	cache, err := repository.New(metadata, objects, cfg.Cache.Path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	repo, err := service.New(metadata, nil, cfg.HTTP.PublicURL).GetRepo(ctx, types.AccountID(*account), *namespace, *name)
	if err == nil {
		err = cache.Compact(ctx, repo)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
