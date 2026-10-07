package config

import (
	"fmt"
	"os"
	"strconv"
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
	Uploads  Uploads
}

type HTTP struct {
	Addr              string
	PublicURL         string
	StreamIdleTimeout time.Duration
	ShutdownTimeout   time.Duration
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
	SSE          string
	SSEKMSKeyID  string
}

type Postgres struct {
	DSN              string
	SkipMigrations   bool
	Auth             string
	Region           string
	Schema           string
	MigrationTimeout time.Duration
}

type Account struct {
	DefaultID string
}

func Load() (Config, error) {
	cfg, err := load()
	if err != nil {
		return cfg, err
	}
	return cfg, cfg.validate(false)
}

func LoadNoAuth() (Config, error) {
	cfg, err := load()
	if err != nil {
		return cfg, err
	}
	cfg.Auth.Mode = authNone
	cfg.Auth.APIToken = ""
	return cfg, cfg.validate(true)
}

func load() (Config, error) {
	uploads, err := loadUploads()
	if err != nil {
		return Config{}, err
	}
	skip, err := strconv.ParseBool(env("ARTIFACTS_SKIP_MIGRATIONS", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("ARTIFACTS_SKIP_MIGRATIONS must be true or false")
	}
	database, err := loadDatabase()
	if err != nil {
		return Config{}, err
	}
	database.SkipMigrations = skip
	shutdown := durationEnv("ARTIFACTS_SHUTDOWN_TIMEOUT", 30*time.Second)
	if shutdown <= 0 {
		return Config{}, fmt.Errorf("ARTIFACTS_SHUTDOWN_TIMEOUT must be a positive duration")
	}
	return Config{
		Uploads: uploads,
		HTTP: HTTP{
			Addr:              env("ARTIFACTS_HTTP_ADDR", defaultAddr),
			PublicURL:         strings.TrimRight(env("ARTIFACTS_PUBLIC_URL", defaultURL), "/"),
			StreamIdleTimeout: durationEnv("ARTIFACTS_STREAM_IDLE_TIMEOUT", 30*time.Second),
			ShutdownTimeout:   shutdown,
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
				SSE:          os.Getenv("S3_SSE"),
				SSEKMSKeyID:  os.Getenv("S3_SSE_KMS_KEY_ID"),
			},
		},
		Cache:    Cache{Path: env("ARTIFACTS_CACHE_DIR", "./cache")},
		Postgres: database,
		Account:  Account{DefaultID: env("ARTIFACTS_DEFAULT_ACCOUNT", defaultAcct)},
	}, nil
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
		if err := c.Storage.S3.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("ARTIFACTS_STORAGE must be %q or %q", backendFS, backendS3)
	}
	if err := c.HTTP.validate(); err != nil {
		return err
	}
	if err := c.Uploads.Defaults().validate(); err != nil {
		return err
	}
	if c.Cache.Path == "" {
		return fmt.Errorf("ARTIFACTS_CACHE_DIR is required")
	}
	if c.Account.DefaultID == "" {
		return fmt.Errorf("ARTIFACTS_DEFAULT_ACCOUNT is required")
	}
	return c.Postgres.Validate()
}

func (h HTTP) validate() error {
	if h.Addr == "" {
		return fmt.Errorf("ARTIFACTS_HTTP_ADDR is required")
	}
	if h.PublicURL == "" {
		return fmt.Errorf("ARTIFACTS_PUBLIC_URL is required")
	}
	if h.ShutdownTimeout < 0 {
		return fmt.Errorf("ARTIFACTS_SHUTDOWN_TIMEOUT must be a positive duration")
	}
	if h.StreamIdleTimeout <= 0 {
		return fmt.Errorf("ARTIFACTS_STREAM_IDLE_TIMEOUT must be a positive duration")
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

func LoadDatabase() (Postgres, error) {
	cfg, err := loadDatabase()
	if err != nil {
		return cfg, err
	}
	if cfg.DSN == "" {
		return Postgres{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}
