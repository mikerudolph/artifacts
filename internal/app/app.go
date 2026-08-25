package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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

// Handler builds the combined REST + git HTTP handler.
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
	if err := postgres.Migrate(cfg.Postgres.DSN); err != nil {
		return nil, err
	}
	mdb, err := postgres.Open(ctx, cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}
	objs, err := openObjects(ctx, cfg)
	if err != nil {
		return nil, err
	}
	svc := service.New(mdb, nil, cfg.HTTP.PublicURL)
	cachePath := cfg.Cache.Path
	if cachePath == "" {
		cachePath = cfg.Storage.FS.Path + ".cache"
	}
	cache, err := repository.New(mdb, objs, cachePath)
	if err != nil {
		return nil, err
	}
	runner := jobs.NewWithPublisher(mdb, objs, cfg.HTTP.PublicURL, cache)
	reader := func(ctx context.Context, account, ns, name string, visit func(storer.Storer) error) error {
		repo, err := svc.GetRepo(ctx, types.AccountID(account), ns, name)
		if err != nil {
			return err
		}
		return cache.Read(ctx, repo, visit)
	}
	if !dev && cfg.Auth.Mode == "token" && cfg.Auth.APIToken != "" {
		if err := ensureAPIToken(ctx, mdb, types.AccountID(cfg.Account.DefaultID), cfg.Auth.APIToken); err != nil {
			return nil, err
		}
	}
	rest := api.NewWithDependencies(svc, cfg, api.Dependencies{
		ReadGit: reader, Jobs: runner, Repository: cache, Authorizer: auth.NewControlAuthorizer(mdb.APITokens()),
		StreamIdle: cfg.HTTP.StreamIdleTimeout,
	})
	git := githttp.NewRepositoryWithIdleTimeout(cache, svc, auth.NewRepoAuthorizer(mdb.RepoTokens(), time.Now), dev, cfg.HTTP.StreamIdleTimeout)
	browser, err := devBrowser(dev, svc, cache)
	if err != nil {
		return nil, err
	}
	h := combinedHandler(rest, git, browser, dev)
	if dev {
		h = devHostGuard(h)
	}
	return h, nil
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

func devBrowser(dev bool, services *service.Services, cache *repository.Manager) (http.Handler, error) {
	if !dev {
		return nil, nil
	}
	return ui.New(services, cache)
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

func ensureAPIToken(ctx context.Context, metadata meta.Store, account types.AccountID, plaintext string) error {
	if err := metadata.Accounts().Ensure(ctx, account); err != nil {
		return err
	}
	_, err := metadata.APITokens().GetByHash(ctx, auth.HashAPI(plaintext))
	if err == nil {
		return nil
	}
	if !meta.IsNotFound(err) {
		return err
	}
	_, err = metadata.APITokens().Create(ctx, types.APIToken{AccountID: account, Hash: auth.HashAPI(plaintext)})
	return err
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

// WriteUsage prints CLI help.
func WriteUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `artifacts — cloud-agnostic versioned git storage

Usage:
  artifacts serve
  artifacts dev [--addr 127.0.0.1:8080]
  artifacts migrate
  artifacts token create --account ACCOUNT
  artifacts compact --account ACCOUNT --namespace NAMESPACE --repo REPO
`)
}
