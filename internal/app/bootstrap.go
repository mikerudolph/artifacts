package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/types"
)

func runBootstrap(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	set.SetOutput(stderr)
	account := set.String("account", "", "account")
	if err := set.Parse(args); err != nil {
		return 2
	}
	if _, err := types.ParseNamespaceName(*account); err != nil || set.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "bootstrap requires --account with a valid account name")
		return 2
	}
	if err := bootstrap(ctx, types.AccountID(*account)); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "Account and control token are ready.")
	return 0
}

func bootstrap(ctx context.Context, account types.AccountID) error {
	plain, err := bootstrapSecret()
	if err != nil {
		return err
	}
	cfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	metadata, err := postgres.Open(ctx, cfg.DSN)
	if err != nil {
		return err
	}
	defer closeMetadata(metadata)
	if err := metadata.(interface{ CheckSchema(context.Context) error }).CheckSchema(ctx); err != nil {
		return err
	}
	return ensureAPIToken(ctx, metadata, account, plain)
}

func bootstrapSecret() (string, error) {
	value, path := os.Getenv("ARTIFACTS_BOOTSTRAP_TOKEN"), os.Getenv("ARTIFACTS_BOOTSTRAP_TOKEN_FILE")
	if value != "" && path != "" {
		return "", fmt.Errorf("set only one bootstrap token source")
	}
	if path != "" {
		file, err := os.Open(path) //nolint:gosec
		if err != nil {
			return "", fmt.Errorf("cannot open bootstrap token file")
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(io.LimitReader(file, 4097))
		if err != nil {
			return "", fmt.Errorf("cannot read bootstrap token file")
		}
		value = string(data)
	}
	if len(value) > 4096 {
		return "", fmt.Errorf("bootstrap token exceeds 4096 bytes")
	}
	value = strings.TrimSpace(value)
	if len(value) < 32 || strings.ContainsAny(value, "\r\n\t ") {
		return "", fmt.Errorf("bootstrap token must contain 32 to 4096 non-whitespace bytes")
	}
	return value, nil
}

func ensureAPIToken(ctx context.Context, metadata meta.Store, account types.AccountID, plaintext string) error {
	return metadata.(interface {
		EnsureAPIToken(context.Context, types.AccountID, string) error
	}).EnsureAPIToken(ctx, account, auth.HashAPI(plaintext))
}

func closeMetadata(metadata meta.Store) {
	if closer, ok := metadata.(interface{ Close() }); ok {
		closer.Close()
	}
}
