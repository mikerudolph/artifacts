package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mikerudolph/artifacts/internal/config"
)

func isolateAWS(t *testing.T) {
	t.Helper()
	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE", "AWS_SESSION_TOKEN", "PGPASSWORD", "PGSERVICE", "PGSERVICEFILE"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/credentials")
	t.Setenv("PGPASSFILE", t.TempDir()+"/pgpass")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "local-test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "local-test-secret")
}

func TestIAMConfigurationAndTLS(t *testing.T) {
	isolateAWS(t)
	for _, dsn := range []string{
		"postgres://worker@db.example/app?sslmode=disable",
		"postgres://worker@db.example/app?sslmode=require",
		"postgres://worker@db.example/app?sslmode=verify-ca",
		"postgres://worker:secret@db.example/app?sslmode=verify-full",
		"postgres://worker@127.0.0.1/app?sslmode=verify-full",
		"host=/tmp user=worker sslmode=verify-full",
		"postgres://worker@one.example,two.example/app?sslmode=verify-full",
	} {
		if _, err := connectionConfig(t.Context(), config.Postgres{DSN: dsn, Auth: "rds-iam", Region: "us-west-2"}); err == nil {
			t.Fatal("unsafe IAM connection accepted")
		}
	}
	cfg := config.Postgres{DSN: "postgres://worker@db.example:5433/app?sslmode=verify-full", Auth: "rds-iam"}
	if _, err := connectionConfig(t.Context(), cfg); err == nil {
		t.Fatal("missing region accepted")
	}
	t.Setenv("AWS_REGION", "us-east-1")
	for _, region := range []string{"", "us-west-2"} {
		cfg.Region = region
		pool, err := connectionConfig(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := pool.BeforeConnect(t.Context(), pool.ConnConfig); err != nil {
			t.Fatal(err)
		}
		want := region
		if want == "" {
			want = "us-east-1"
		}
		assertIAMToken(t, pool.ConnConfig.Password, "local-test-key", want)
	}
}

func assertIAMToken(t *testing.T, token, key, region string) {
	t.Helper()
	u, err := url.Parse("https://" + token)
	if err != nil {
		t.Fatal("invalid signed token")
	}
	q := u.Query()
	if q.Get("DBUser") != "worker" || q.Get("Action") != "connect" || q.Get("X-Amz-Expires") != "900" || !strings.Contains(q.Get("X-Amz-Credential"), key+"/") || !strings.Contains(q.Get("X-Amz-Credential"), "/"+region+"/rds-db/") || q.Get("X-Amz-Signature") == "" {
		t.Fatal("incorrect IAM signing fields")
	}
}

func TestIAMRefreshAndRedactedFailures(t *testing.T) {
	var calls atomic.Int32
	provider := aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: fmt.Sprintf("key-%d", calls.Add(1)), SecretAccessKey: "secret", SessionToken: "session", CanExpire: true, Expires: time.Now().Add(-time.Minute)}, nil
	}))
	hook := iamBeforeConnect("us-west-2", provider)
	conn, err := pgx.ParseConfig("postgres://worker@db.example:5433/app?sslmode=verify-full")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := hook(t.Context(), conn); err != nil {
			t.Fatal(err)
		}
		assertIAMToken(t, conn.Password, fmt.Sprintf("key-%d", i), "us-west-2")
		u, _ := url.Parse("https://" + conn.Password)
		if u.Host != "db.example:5433" || u.Query().Get("X-Amz-Security-Token") != "session" {
			t.Fatal("endpoint or session signing")
		}
	}
	failing := iamBeforeConnect("us-west-2", aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{}, errors.New("private credential details")
	}))
	if err := failing(t.Context(), conn); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("credential error disclosure")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := failing(ctx, conn); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation")
	}
	for _, err := range []error{errors.New(conn.Password), &pgconn.PgError{Code: "28P01", Message: conn.Password}, fmt.Errorf("%s: %w", conn.Password, context.DeadlineExceeded)} {
		if strings.Contains(databaseError(err).Error(), conn.Password) {
			t.Fatal("database error disclosed token")
		}
	}
}

func TestDatabaseSelectionValidation(t *testing.T) {
	for _, cfg := range []config.Postgres{
		{DSN: "postgres://user:secret@host:invalid/db"},
		{DSN: "postgres://host/db?search_path=public", Schema: "artifacts"},
		{Schema: "invalid-schema"},
	} {
		if _, err := OpenConfigured(t.Context(), cfg); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid config or disclosure")
		}
	}
	for _, path := range []string{"", "artifacts", `"artifacts",pg_temp`} {
		cfg, err := connectionConfig(t.Context(), config.Postgres{DSN: "postgres://localhost/db?pool_max_conns=3&search_path=" + url.QueryEscape(path), Schema: "artifacts"})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MaxConns != 3 || cfg.ConnConfig.RuntimeParams["search_path"] != `"artifacts",pg_temp` {
			t.Fatal("pool or schema settings")
		}
	}
}
