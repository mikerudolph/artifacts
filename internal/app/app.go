package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/api"
	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/githttp"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/repository"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/fs"
	objs3 "github.com/mikerudolph/artifacts/internal/store/object/s3"
	"github.com/mikerudolph/artifacts/internal/types"
	"github.com/mikerudolph/artifacts/internal/ui"
)

func Handler(ctx context.Context, cfg config.Config) (http.Handler, error) {
	return buildHandler(ctx, cfg, false)
}

func buildHandler(ctx context.Context, cfg config.Config, dev bool) (http.Handler, error) {
	if dev {
		cfg.Auth.Mode = "none"
		cfg.Auth.APIToken = ""
	} else if cfg.Auth.Mode == "none" {
		return nil, fmt.Errorf("no-auth mode is restricted to artifacts dev")
	}
	mdb, err := openMetadata(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	built := false
	defer func() {
		if !built {
			closeMetadata(mdb)
		}
	}()
	checkSchema := mdb.(interface{ CheckSchema(context.Context) error }).CheckSchema
	objs, err := openObjects(ctx, cfg)
	if err != nil {
		return nil, err
	}
	svc := service.New(mdb, nil, cfg.HTTP.PublicURL)
	cache, err := openCache(mdb, objs, cfg)
	if err != nil {
		return nil, err
	}
	runner := jobs.NewWithPublisher(mdb, objs, cfg.HTTP.PublicURL, cache)
	reader := func(ctx context.Context, account, ns, name string, visit func(storer.Storer) error) error {
		repo, err := svc.GetRepo(ctx, types.AccountID(account), ns, name)
		if err != nil {
			return err
		}
		return cache.ReadContent(ctx, repo, visit)
	}
	if !dev && cfg.Auth.Mode == "token" && cfg.Auth.APIToken != "" {
		if err := ensureAPIToken(ctx, mdb, types.AccountID(cfg.Account.DefaultID), cfg.Auth.APIToken); err != nil {
			return nil, err
		}
	}
	rest := api.NewWithDependencies(svc, cfg, api.Dependencies{
		ReadGit: reader, Jobs: runner, Repository: cache, Authorizer: auth.NewControlAuthorizer(mdb.APITokens()),
		StreamIdle:     cfg.HTTP.StreamIdleTimeout,
		RepoAuthorizer: auth.NewRepoAuthorizer(mdb.RepoTokens(), time.Now),
	})
	git := githttp.NewRepositoryWithIdleTimeout(cache, svc, auth.NewRepoAuthorizer(mdb.RepoTokens(), time.Now), dev, cfg.HTTP.StreamIdleTimeout)
	browser, err := devBrowser(dev, rest)
	if err != nil {
		return nil, err
	}
	h := combinedHandler(rest, git, browser, dev)
	if dev {
		h = devHostGuard(h)
	}
	built = true
	return &maintainedHandler{Handler: h, manager: cache, shutdownTimeout: cfg.HTTP.ShutdownTimeout,
		close: func() { closeMetadata(mdb) },
		ready: func(ctx context.Context) error {
			if err := checkSchema(ctx); err != nil {
				return err
			}
			_, err := objs.Exists(ctx, "health/readiness")
			return err
		},
	}, nil
}

func devHostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
		if !loopbackHost(host) {
			http.Error(w, "invalid development host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func devBrowser(dev bool, rest http.Handler) (http.Handler, error) {
	if !dev {
		return nil, nil
	}
	return ui.New(rest)
}

func combinedHandler(rest, git, browser http.Handler, dev bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/git/") {
			git.ServeHTTP(w, r)
			return
		}
		if dev && !strings.HasPrefix(r.URL.Path, "/client/") {
			browser.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

func openObjects(ctx context.Context, cfg config.Config) (object.Store, error) {
	if cfg.Storage.Backend == "s3" {
		return objs3.New(ctx, cfg.Storage.S3)
	}
	return fs.New(cfg.Storage.FS.Path)
}

type tokenLookup struct{ meta meta.Store }

func (t tokenLookup) Lookup(ctx context.Context, _, _, plaintext string) (types.Scope, error) {
	secret, _, err := auth.ParseRepo(plaintext)
	if err != nil {
		return "", err
	}
	tok, err := t.meta.RepoTokens().GetByHash(ctx, auth.HashRepo(secret))
	if err != nil {
		return "", err
	}
	if tok.State != types.TokenActive || !tok.ExpiresAt.After(time.Now()) {
		return "", auth.ErrExpired
	}
	return tok.Scope, nil
}

func WriteUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `artifacts — cloud-agnostic versioned git storage

Usage:
  artifacts serve
  artifacts dev [--addr 127.0.0.1:8080]
  artifacts migrate
  artifacts bootstrap --account ACCOUNT
  artifacts token create --account ACCOUNT
  artifacts compact --account ACCOUNT --namespace NAMESPACE --repo REPO
`)
}

func openMetadata(ctx context.Context, cfg config.Postgres) (meta.V2Store, error) {
	if !cfg.SkipMigrations {
		if err := postgres.Migrate(cfg.DSN); err != nil {
			return nil, err
		}
	}
	mdb, err := postgres.Open(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := mdb.(interface{ CheckSchema(context.Context) error }).CheckSchema(ctx); err != nil {
		closeMetadata(mdb)
		return nil, err
	}
	return mdb, nil
}

func openCache(mdb meta.V2Store, objs object.Store, cfg config.Config) (*repository.Manager, error) {
	path := cfg.Cache.Path
	if path == "" {
		path = cfg.Storage.FS.Path + ".cache"
	}
	if err := writableDirectory(path); err != nil {
		return nil, err
	}
	if err := writableDirectory(os.TempDir()); err != nil {
		return nil, err
	}
	return repository.New(mdb, objs, path)
}
