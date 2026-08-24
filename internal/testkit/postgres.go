package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Postgres starts a Postgres 16 container and returns a DSN. Skips without Docker.
func Postgres(tb testing.TB) string {
	tb.Helper()
	DockerAvailable(tb)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("artifacts"),
		postgres.WithUsername("artifacts"),
		postgres.WithPassword("artifacts"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		tb.Fatalf("postgres container: %v", err)
	}
	tb.Cleanup(func() {
		_ = ctr.Terminate(context.Background())
	})
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		tb.Fatalf("postgres dsn: %v", err)
	}
	return dsn
}
