package testkit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func localPostgres(tb testing.TB, dsn string) string {
	tb.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		tb.Fatal("ARTIFACTS_TEST_POSTGRES_URL must be a PostgreSQL URL for an owned test server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		tb.Fatal("connect to owned PostgreSQL test server:", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		tb.Fatal(err)
	}
	name := "artifacts_test_" + hex.EncodeToString(suffix[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		tb.Fatal("create owned test database:", err)
	}
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			tb.Error("reconnect for owned database cleanup:", err)
			return
		}
		defer func() { _ = admin.Close(context.Background()) }()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			tb.Error("remove owned test database:", err)
		}
	})
	parsed.Path = "/" + name
	query := parsed.Query()
	query.Del("dbname")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
