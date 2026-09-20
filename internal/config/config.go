package config

import (
	"fmt"
	"os"
	"strings"
	"time"
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

type Config struct {
	HTTP     HTTP
	Auth     Auth
	Storage  Storage
	Cache    Cache
	Postgres Postgres
	Account  Account
}

type HTTP struct {
	Addr              string
	PublicURL         string
	StreamIdleTimeout time.Duration
}

type Auth struct {
	Mode     string
	APIToken string
}

type Storage struct {
	Backend string
	FS      FS
	S3      S3
}

type Cache struct {
	Path string
}

type FS struct {
	Path string
}

type S3 struct {
	Endpoint     string
	Bucket       string
	Region       string
	AccessKey    string
	SecretKey    string
	Prefix       string
	UsePathStyle bool
}

type Postgres struct {
	DSN string
}

type Account struct {
	DefaultID string
}

func Load() (Config, error) {
	cfg := load()
	return cfg, cfg.validate(false)
}

func LoadNoAuth() (Config, error) {
	cfg := load()
	cfg.Auth.Mode = authNone
	cfg.Auth.APIToken = ""
	return cfg, cfg.validate(true)
}

func load() Config {
	return Config{
		HTTP: HTTP{
			Addr:              env("ARTIFACTS_HTTP_ADDR", defaultAddr),
			PublicURL:         strings.TrimRight(env("ARTIFACTS_PUBLIC_URL", defaultURL), "/"),
			StreamIdleTimeout: durationEnv("ARTIFACTS_STREAM_IDLE_TIMEOUT", 30*time.Second),
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
		Cache:    Cache{Path: env("ARTIFACTS_CACHE_DIR", "./cache")},
		Postgres: Postgres{DSN: firstEnv("DATABASE_URL", "ARTIFACTS_DATABASE_URL")},
		Account:  Account{DefaultID: env("ARTIFACTS_DEFAULT_ACCOUNT", defaultAcct)},
	}
}

func (c Config) Validate() error {
	return c.validate(false)
}

func (c Config) validate(allowNoAuth bool) error {
	switch c.Auth.Mode {
	case authToken:
	case authNone:
		if !allowNoAuth {
			return fmt.Errorf("ARTIFACTS_AUTH=none is restricted to artifacts dev")
		}
	default:
		return fmt.Errorf("ARTIFACTS_AUTH must be %q or %q", authToken, authNone)
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
	if c.HTTP.StreamIdleTimeout <= 0 {
		return fmt.Errorf("ARTIFACTS_STREAM_IDLE_TIMEOUT must be a positive duration")
	}
	if c.Cache.Path == "" {
		return fmt.Errorf("ARTIFACTS_CACHE_DIR is required")
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

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return duration
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
