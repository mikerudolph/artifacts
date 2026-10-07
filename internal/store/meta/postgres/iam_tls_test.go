package postgres

import (
	"context"
	"database/sql"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mikerudolph/artifacts/internal/config"
)

func TestIAMTLSModesOnWire(t *testing.T) {
	isolateAWS(t)
	for _, tc := range []struct {
		mode, host string
		ca, valid  bool
	}{
		{"require", "wrong.example", false, true},
		{"require", "wrong.example", true, true},
		{"verify-ca", "wrong.example", true, true},
		{"verify-full", "example.com", true, true},
		{"verify-ca", "example.com", false, false},
		{"verify-full", "example.com", false, false},
		{"verify-full", "wrong.example", true, false},
	} {
		t.Run(tc.mode+"-"+tc.host+"-ca="+map[bool]string{true: "yes", false: "no"}[tc.ca], func(t *testing.T) {
			tokens, address, _, ca := iamWireServer(t, !tc.valid)
			dsn := "postgres://worker@" + tc.host + "/app?sslmode=" + tc.mode
			if tc.ca {
				dsn += "&sslrootcert=" + url.QueryEscape(ca)
			}
			pool, err := connectionConfig(t.Context(), config.Postgres{DSN: dsn, Auth: "rds-iam", Region: "us-west-2"})
			if err != nil {
				t.Fatal(err)
			}
			pool.ConnConfig.DialFunc = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}
			pool.ConnConfig.LookupFunc = func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }
			for _, sqlDriver := range []bool{false, true} {
				err := connectIAMTLS(t.Context(), pool.ConnConfig, pool.BeforeConnect, sqlDriver)
				if (err == nil) != tc.valid {
					t.Fatal("unexpected TLS acceptance", err)
				}
				if tc.valid {
					select {
					case token := <-tokens:
						assertIAMToken(t, token, "local-test-key", "us-west-2")
					case <-time.After(time.Second):
						t.Fatal("missing signed token over TLS")
					}
				}
			}
			if len(tokens) != 0 {
				t.Fatal("unexpected authentication attempt")
			}
		})
	}
}

func connectIAMTLS(ctx context.Context, cfg *pgx.ConnConfig, hook func(context.Context, *pgx.ConnConfig) error, sqlDriver bool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if sqlDriver {
		db := sql.OpenDB(stdlib.GetConnector(*cfg, stdlib.OptionBeforeConnect(hook)))
		defer func() { _ = db.Close() }()
		conn, err := db.Conn(ctx)
		if err == nil {
			_ = conn.Close()
		}
		return err
	}
	copy := cfg.Copy()
	if err := hook(ctx, copy); err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, copy)
	if err == nil {
		_ = conn.Close(ctx)
	}
	return err
}

func TestIAMTLSWarning(t *testing.T) {
	if mode := os.Getenv("ARTIFACTS_TEST_IAM_WARNING"); mode != "" {
		isolateAWS(t)
		for range 3 {
			_, err := connectionConfig(t.Context(), config.Postgres{DSN: "postgres://worker@db.example/app?sslmode=" + mode, Auth: "rds-iam", Region: "us-west-2"})
			if err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	for _, mode := range []string{"require", "verify-ca", "verify-full"} {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestIAMTLSWarning$") //nolint:gosec
		cmd.Env = append(os.Environ(), "ARTIFACTS_TEST_IAM_WARNING="+mode)
		output, err := cmd.CombinedOutput()
		want := 1
		if mode == "verify-full" {
			want = 0
		}
		if err != nil || strings.Count(string(output), "WARN RDS IAM TLS") != want {
			t.Fatal("warning missing, repeated, or emitted for verified TLS", err)
		}
		for _, value := range []string{"worker", "db.example", "local-test-key", "local-test-secret"} {
			if strings.Contains(string(output), value) {
				t.Fatal("startup warning disclosed connection details")
			}
		}
	}
}

func TestIAMRejectsTLSRefusal(t *testing.T) {
	isolateAWS(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	done := make(chan int, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- -1
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.ReadFull(conn, make([]byte, 8))
		_, _ = conn.Write([]byte("N"))
		body, _ := io.ReadAll(conn)
		done <- len(body)
	}()
	pool, err := connectionConfig(t.Context(), config.Postgres{DSN: "postgres://worker@localhost:" + strings.Split(listener.Addr().String(), ":")[1] + "/app?sslmode=require", Auth: "rds-iam", Region: "us-west-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := connectIAMTLS(t.Context(), pool.ConnConfig, pool.BeforeConnect, false); err == nil || <-done != 0 {
		t.Fatal("TLS refusal allowed plaintext authentication")
	}
}

func TestIAMTLSModeFromEnvironmentAndService(t *testing.T) {
	isolateAWS(t)
	service := t.TempDir() + "/pg_service.conf"
	t.Setenv("PGSERVICEFILE", service)
	for _, mode := range []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"} {
		t.Setenv("PGSSLMODE", mode)
		valid := mode == "require" || mode == "verify-ca" || mode == "verify-full"
		for _, dsn := range []string{"postgres://worker@db.example/app?sslrootcert=system", "host=db.example user=worker sslrootcert=system"} {
			_, err := connectionConfig(t.Context(), config.Postgres{DSN: dsn, Auth: "rds-iam", Region: "us-west-2"})
			if (err == nil) != valid {
				t.Fatal("environment TLS policy lost", err)
			}
		}
		t.Setenv("PGSSLMODE", "")
		if err := os.WriteFile(service, []byte("[test]\nhost=db.example\nuser=worker\nsslmode="+mode+"\nsslnegotiation=direct\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := connectionConfig(t.Context(), config.Postgres{DSN: "service=test", Auth: "rds-iam", Region: "us-west-2"}); (err == nil) != valid {
			t.Fatal("service TLS policy lost", err)
		}
	}
}
