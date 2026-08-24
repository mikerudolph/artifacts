package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	backendFS   = "fs"
	backendS3   = "s3"
	authToken   = "token"
	authNone    = "none"
	defaultAddr = ":8080"
	defaultURL  = "http://localhost:8080"
	defaultFS   = "./data"
	defaultAcct = "local"
)

// Config is process configuration loaded from the environment.
type Config struct {
	HTTP     HTTP
	Auth     Auth
	Storage  Storage
	Postgres Postgres
	Account  Account
}

// HTTP is the listen address and public git/API base URL.
type HTTP struct {
	Addr      string
	PublicURL string
}

// Auth is control-plane authentication.
type Auth struct {
	Mode     string
	APIToken string
}

// Storage selects the object backend.
type Storage struct {
	Backend string
	FS      FS
	S3      S3
}

// FS is the local filesystem object backend.
type FS struct {
	Path string
}

// S3 is any S3-compatible object backend.
type S3 struct {
	Endpoint     string
	Bucket       string
	Region       string
	AccessKey    string
	SecretKey    string
	Prefix       string
	UsePathStyle bool
}

// Postgres is the metadata database.
type Postgres struct {
	DSN string
}

// Account holds the default tenant used when none is supplied.
type Account struct {
	DefaultID string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	cfg := Config{
		HTTP: HTTP{
			Addr:      env("ARTIFACTS_HTTP_ADDR", defaultAddr),
			PublicURL: strings.TrimRight(env("ARTIFACTS_PUBLIC_URL", defaultURL), "/"),
		},
		Auth: Auth{
			Mode:     env("ARTIFACTS_AUTH", authToken),
			APIToken: os.Getenv("ARTIFACTS_API_TOKEN"),
		},
		Storage: Storage{
			Backend: env("ARTIFACTS_STORAGE", backendFS),
			FS:      FS{Path: env("ARTIFACTS_DATA_DIR", defaultFS)},
			S3: S3{
				Endpoint:     firstEnv("S3_ENDPOINT", "AWS_ENDPOINT_URL"),
				Bucket:       os.Getenv("S3_BUCKET"),
				Region:       env("S3_REGION", env("AWS_REGION", "us-east-1")),
				AccessKey:    firstEnv("AWS_ACCESS_KEY_ID", "S3_ACCESS_KEY"),
				SecretKey:    firstEnv("AWS_SECRET_ACCESS_KEY", "S3_SECRET_KEY"),
				Prefix:       os.Getenv("S3_PREFIX"),
				UsePathStyle: truthy(os.Getenv("S3_USE_PATH_STYLE")),
			},
		},
		Postgres: Postgres{DSN: firstEnv("DATABASE_URL", "ARTIFACTS_DATABASE_URL")},
		Account:  Account{DefaultID: env("ARTIFACTS_DEFAULT_ACCOUNT", defaultAcct)},
	}
	return cfg, cfg.Validate()
}

// Validate reports configuration that cannot start the server.
func (c Config) Validate() error {
	switch c.Auth.Mode {
	case authToken, authNone:
	default:
		return fmt.Errorf("ARTIFACTS_AUTH must be %q or %q", authToken, authNone)
	}
	if c.Auth.Mode == authToken && c.Auth.APIToken == "" {
		return fmt.Errorf("ARTIFACTS_API_TOKEN is required when ARTIFACTS_AUTH=token")
	}
	switch c.Storage.Backend {
	case backendFS:
		if c.Storage.FS.Path == "" {
			return fmt.Errorf("ARTIFACTS_DATA_DIR is required when ARTIFACTS_STORAGE=fs")
		}
	case backendS3:
		if c.Storage.S3.Bucket == "" {
			return fmt.Errorf("S3_BUCKET is required when ARTIFACTS_STORAGE=s3")
		}
	default:
		return fmt.Errorf("ARTIFACTS_STORAGE must be %q or %q", backendFS, backendS3)
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("ARTIFACTS_HTTP_ADDR is required")
	}
	if c.HTTP.PublicURL == "" {
		return fmt.Errorf("ARTIFACTS_PUBLIC_URL is required")
	}
	if c.Account.DefaultID == "" {
		return fmt.Errorf("ARTIFACTS_DEFAULT_ACCOUNT is required")
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
