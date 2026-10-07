package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestIAMSignsEachPhysicalConnection(t *testing.T) {
	tokens, addr, roots, _ := iamWireServer(t, false)
	settings, err := pgxpool.ParseConfig("postgres://worker@example.com:5432/app?sslmode=verify-full")
	if err != nil {
		t.Fatal(err)
	}
	settings.ConnConfig.TLSConfig.RootCAs = roots
	settings.ConnConfig.DialFunc = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	settings.ConnConfig.LookupFunc = func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	var calls atomic.Int32
	settings.BeforeConnect = iamBeforeConnect("us-west-2", aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: fmt.Sprintf("key-%d", calls.Add(1)), SecretAccessKey: "test-secret"}, nil
	}))
	pool, err := pgxpool.NewWithConfig(t.Context(), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for i := 1; i <= 3; i++ {
		conn, err := pool.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		assertIAMToken(t, <-tokens, fmt.Sprintf("key-%d", i), "us-west-2")
		if err := conn.Conn().Close(t.Context()); err != nil {
			t.Fatal(err)
		}
		conn.Release()
	}
	db := sql.OpenDB(migrationConnector{Connector: stdlib.GetConnector(*settings.ConnConfig, stdlib.OptionBeforeConnect(connectionHook(settings.BeforeConnect))), ctx: t.Context()})
	defer func() { _ = db.Close() }()
	db.SetMaxIdleConns(0)
	for i := 4; i <= 6; i++ {
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		assertIAMToken(t, <-tokens, fmt.Sprintf("key-%d", i), "us-west-2")
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if settings.ConnConfig.Password != "" {
		t.Fatal("token mutated reusable configuration")
	}
}

func iamWireServer(t *testing.T, rejectTLS bool) (<-chan string, string, *x509.CertPool, string) {
	t.Helper()
	sample := httptest.NewTLSServer(nil)
	tlsConfig := sample.TLS.Clone()
	roots := x509.NewCertPool()
	roots.AddCert(sample.Certificate())
	ca := t.TempDir() + "/ca.pem"
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sample.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sample.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tokens := make(chan string, 10)
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer func() { _ = conn.Close() }()
				if err := serveIAMLogin(conn, tlsConfig, tokens); err != nil && !rejectTLS {
					t.Error(err)
				}
			})
		}
	})
	t.Cleanup(func() { _ = listener.Close(); wg.Wait() })
	return tokens, listener.Addr().String(), roots, ca
}

func serveIAMLogin(conn net.Conn, settings *tls.Config, tokens chan<- string) error {
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn, make([]byte, 8)); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("S")); err != nil {
		return err
	}
	secure := tls.Server(conn, settings)
	backend := pgproto3.NewBackend(secure, secure)
	if _, err := backend.ReceiveStartupMessage(); err != nil {
		return err
	}
	backend.Send(&pgproto3.AuthenticationCleartextPassword{})
	if err := backend.Flush(); err != nil {
		return err
	}
	if err := backend.SetAuthType(pgproto3.AuthTypeCleartextPassword); err != nil {
		return err
	}
	message, err := backend.Receive()
	if err != nil {
		return err
	}
	password, ok := message.(*pgproto3.PasswordMessage)
	if !ok {
		return fmt.Errorf("expected password message")
	}
	tokens <- password.Password
	backend.Send(&pgproto3.AuthenticationOk{})
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return err
	}
	_, err = backend.Receive()
	return err
}
