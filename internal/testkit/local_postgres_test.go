package testkit

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestLocalPostgresIsolation(t *testing.T) {
	base := Postgres(t)
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("dbname", strings.TrimPrefix(parsed.Path, "/"))
	parsed.RawQuery = query.Encode()
	t.Setenv("ARTIFACTS_TEST_POSTGRES_URL", parsed.String())
	one := Postgres(t)
	two := Postgres(t)
	if one == two || one == base {
		t.Fatal("test databases share identity")
	}
	conn, err := pgx.Connect(t.Context(), one)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(t.Context(), "CREATE TABLE isolated(value int)"); err != nil {
		t.Fatal(err)
	}
	other, err := pgx.Connect(t.Context(), two)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close(context.Background()) }()
	if _, err := other.Exec(t.Context(), "SELECT * FROM isolated"); err == nil {
		t.Fatal("neighboring database table visible")
	}
}

type fixtureFailure struct{ message string }

type fixtureReporter struct{ testing.TB }

func (fixtureReporter) Fatal(args ...any) {
	panic(fixtureFailure{fmt.Sprint(args...)})
}

func TestPostgresFixtureRejectsInvalidConfiguration(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, url, diagnostic string
	}{
		{"wrong-scheme", "https://127.0.0.1/postgres", "must be a PostgreSQL URL"},
		{"unreachable", "postgres://artifacts@" + address + "/postgres?sslmode=disable", "connect to owned PostgreSQL test server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARTIFACTS_TEST_POSTGRES_URL", tc.url)
			defer func() {
				failure, ok := recover().(fixtureFailure)
				if !ok || !strings.Contains(failure.message, tc.diagnostic) {
					t.Error("missing fixture failure diagnostic")
				}
			}()
			Postgres(fixtureReporter{TB: t})
			t.Error("accepted invalid test database configuration")
		})
	}
}
