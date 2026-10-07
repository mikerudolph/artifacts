package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

func TestCommandsSendIAMTokenOverRequiredTLS(t *testing.T) {
	configureIAMCommandTest(t)
	for _, args := range [][]string{
		{"serve"}, {"migrate"}, {"bootstrap", "--account", "local"},
		{"token", "create", "--account", "local"}, {"compact", "--repo", "test"},
	} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			t.Setenv("DATABASE_URL", "postgres://worker@localhost:"+port+"/app?sslmode=require&pool_max_conns=1")
			certificate := httptest.NewTLSServer(nil)
			defer certificate.Close()
			tokens := make(chan string, 1)
			go rejectIAMLogin(listener, certificate.TLS, tokens)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var output bytes.Buffer
			if Run(ctx, args, &output, &output) != 1 {
				t.Fatal("expected fixture authentication rejection")
			}
			select {
			case token := <-tokens:
				parsed, err := url.Parse("https://" + token)
				if err != nil || parsed.Query().Get("DBUser") != "worker" || parsed.Query().Get("X-Amz-Signature") == "" {
					t.Fatal("command did not send a signed IAM login over TLS")
				}
				if strings.Contains(output.String(), token) {
					t.Fatal("command disclosed token")
				}
			case <-ctx.Done():
				t.Fatal("command failed before TLS authentication")
			}
		})
	}
}

func configureIAMCommandTest(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"ARTIFACTS_DATABASE_AUTH": "rds-iam", "ARTIFACTS_DATABASE_REGION": "us-east-1", "ARTIFACTS_DATABASE_SCHEMA": "",
		"ARTIFACTS_SKIP_MIGRATIONS": "true", "ARTIFACTS_STORAGE": "fs", "ARTIFACTS_AUTH": "token",
		"ARTIFACTS_BOOTSTRAP_TOKEN": strings.Repeat("x", 48), "ARTIFACTS_BOOTSTRAP_TOKEN_FILE": "",
		"AWS_ACCESS_KEY_ID": "test-key", "AWS_SECRET_ACCESS_KEY": "test-secret", "AWS_SESSION_TOKEN": "",
		"AWS_EC2_METADATA_DISABLED": "true", "AWS_PROFILE": "", "PGPASSWORD": "", "PGSSLROOTCERT": "",
		"PGSERVICE": "", "PGSERVICEFILE": "", "PGSSLNEGOTIATION": "",
		"AWS_CONFIG_FILE": t.TempDir() + "/config", "AWS_SHARED_CREDENTIALS_FILE": t.TempDir() + "/credentials",
		"PGPASSFILE": t.TempDir() + "/pgpass", "ARTIFACTS_CACHE_DIR": t.TempDir(), "ARTIFACTS_DATA_DIR": t.TempDir(),
	} {
		t.Setenv(key, value)
	}
}

func rejectIAMLogin(listener net.Listener, settings *tls.Config, tokens chan<- string) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, make([]byte, 8)); err != nil {
		return
	}
	if _, err := conn.Write([]byte("S")); err != nil {
		return
	}
	secure := tls.Server(conn, settings)
	backend := pgproto3.NewBackend(secure, secure)
	if _, err := backend.ReceiveStartupMessage(); err != nil {
		return
	}
	backend.Send(&pgproto3.AuthenticationCleartextPassword{})
	if backend.Flush() != nil || backend.SetAuthType(pgproto3.AuthTypeCleartextPassword) != nil {
		return
	}
	message, err := backend.Receive()
	if err != nil {
		return
	}
	if password, ok := message.(*pgproto3.PasswordMessage); ok {
		tokens <- password.Password
	}
	backend.Send(&pgproto3.ErrorResponse{Severity: "FATAL", Code: "28P01", Message: "test endpoint rejects login after observing TLS token"})
	_ = backend.Flush()
}
