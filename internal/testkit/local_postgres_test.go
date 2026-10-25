package testkit

import (
	"context"
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
	one := localPostgres(t, parsed.String())
	two := localPostgres(t, parsed.String())
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
