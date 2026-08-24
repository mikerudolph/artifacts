package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ARTIFACTS_API_TOKEN", "secret")
	t.Setenv("ARTIFACTS_HTTP_ADDR", "")
	t.Setenv("ARTIFACTS_PUBLIC_URL", "")
	t.Setenv("ARTIFACTS_AUTH", "")
	t.Setenv("ARTIFACTS_STORAGE", "")
	t.Setenv("ARTIFACTS_DATA_DIR", "")
	t.Setenv("ARTIFACTS_DEFAULT_ACCOUNT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ARTIFACTS_DATABASE_URL", "")
	t.Setenv("S3_BUCKET", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != defaultAddr || cfg.HTTP.PublicURL != defaultURL {
		t.Fatalf("http %+v", cfg.HTTP)
	}
	if cfg.Auth.Mode != authToken || cfg.Auth.APIToken != "secret" {
		t.Fatalf("auth %+v", cfg.Auth)
	}
	if cfg.Storage.Backend != backendFS || cfg.Storage.FS.Path != defaultFS {
		t.Fatalf("storage %+v", cfg.Storage)
	}
	if cfg.Account.DefaultID != defaultAcct {
		t.Fatalf("account %+v", cfg.Account)
	}
}

func TestLoadS3AndTrimPublicURL(t *testing.T) {
	t.Setenv("ARTIFACTS_API_TOKEN", "secret")
	t.Setenv("ARTIFACTS_STORAGE", "s3")
	t.Setenv("S3_BUCKET", "arts")
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("S3_REGION", "us-west-2")
	t.Setenv("AWS_ACCESS_KEY_ID", "ak")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	t.Setenv("S3_PREFIX", "p")
	t.Setenv("S3_USE_PATH_STYLE", "true")
	t.Setenv("ARTIFACTS_PUBLIC_URL", "http://localhost:8080/")
	t.Setenv("DATABASE_URL", "postgres://localhost/artifacts")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.PublicURL != "http://localhost:8080" {
		t.Fatalf("public url %q", cfg.HTTP.PublicURL)
	}
	s3 := cfg.Storage.S3
	if s3.Bucket != "arts" || s3.Endpoint == "" || s3.AccessKey != "ak" || !s3.UsePathStyle {
		t.Fatalf("s3 %+v", s3)
	}
	if cfg.Postgres.DSN != "postgres://localhost/artifacts" {
		t.Fatalf("dsn %q", cfg.Postgres.DSN)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []Config{
		{Auth: Auth{Mode: "ldap"}, HTTP: HTTP{Addr: ":1", PublicURL: "u"}, Storage: Storage{Backend: "fs", FS: FS{Path: "p"}}, Account: Account{DefaultID: "a"}},
		{Auth: Auth{Mode: authToken}, HTTP: HTTP{Addr: ":1", PublicURL: "u"}, Storage: Storage{Backend: "fs", FS: FS{Path: "p"}}, Account: Account{DefaultID: "a"}},
		{Auth: Auth{Mode: authNone}, HTTP: HTTP{Addr: ":1", PublicURL: "u"}, Storage: Storage{Backend: "s3"}, Account: Account{DefaultID: "a"}},
		{Auth: Auth{Mode: authNone}, HTTP: HTTP{Addr: ":1", PublicURL: "u"}, Storage: Storage{Backend: "gcs"}, Account: Account{DefaultID: "a"}},
		{Auth: Auth{Mode: authNone}, HTTP: HTTP{PublicURL: "u"}, Storage: Storage{Backend: "fs", FS: FS{Path: "p"}}, Account: Account{DefaultID: "a"}},
		{Auth: Auth{Mode: authNone}, HTTP: HTTP{Addr: ":1"}, Storage: Storage{Backend: "fs", FS: FS{Path: "p"}}, Account: Account{DefaultID: "a"}},
		{Auth: Auth{Mode: authNone}, HTTP: HTTP{Addr: ":1", PublicURL: "u"}, Storage: Storage{Backend: "fs", FS: FS{Path: "p"}}},
		{Auth: Auth{Mode: authNone}, HTTP: HTTP{Addr: ":1", PublicURL: "u"}, Storage: Storage{Backend: "fs"}, Account: Account{DefaultID: "a"}},
	}
	for i, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestLoadAuthNone(t *testing.T) {
	t.Setenv("ARTIFACTS_AUTH", "none")
	t.Setenv("ARTIFACTS_API_TOKEN", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Mode != authNone {
		t.Fatalf("mode %q", cfg.Auth.Mode)
	}
}

func TestTruthyAndFirstEnv(t *testing.T) {
	t.Setenv("S3_USE_PATH_STYLE", "YES")
	t.Setenv("ARTIFACTS_API_TOKEN", "x")
	t.Setenv("S3_ACCESS_KEY", "from-s3")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !truthy("1") || !truthy("on") || truthy("no") {
		t.Fatal("truthy")
	}
	if cfg.Storage.S3.UsePathStyle != true {
		t.Fatal("path style")
	}
}
