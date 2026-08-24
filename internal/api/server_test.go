package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/testkit"
)

const acctBase = "/client/v4/accounts/local/artifacts"

func testAPI(t *testing.T, mode, token string) http.Handler {
	t.Helper()
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	st, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := st.(interface{ Close() }); ok {
			c.Close()
		}
	})
	svc := service.New(st, time.Now, "http://example.test")
	return New(svc, config.Config{Auth: config.Auth{Mode: mode, APIToken: token}})
}

func doJSON(t *testing.T, h http.Handler, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Errors  []struct {
			Code int `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if dest != nil && env.Result != nil {
		if err := json.Unmarshal(env.Result, dest); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuthRequired(t *testing.T) {
	h := testAPI(t, "token", "secret")
	rec := doJSON(t, h, http.MethodGet, acctBase+"/namespaces", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, acctBase+"/namespaces", "secret", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
}
