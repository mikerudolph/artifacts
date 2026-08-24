package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/api"
	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/githttp"
	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/service"
	gitstore "github.com/mikerudolph/artifacts/internal/store/git"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/fs"
	objs3 "github.com/mikerudolph/artifacts/internal/store/object/s3"
	"github.com/mikerudolph/artifacts/internal/types"
)

// Handler builds the combined REST + git HTTP handler.
func Handler(ctx context.Context, cfg config.Config) (http.Handler, error) {
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
	api.Jobs = jobs.New(mdb, objs, cfg.HTTP.PublicURL)
	acct := types.AccountID(cfg.Account.DefaultID)
	open := func(ns, name string) (storer.Storer, error) {
		repo, err := svc.GetRepo(context.Background(), acct, ns, name)
		if err != nil {
			return nil, err
		}
		return gitstore.Open(objs, mdb.Refs(), acct, repo.ID)
	}
	api.OpenGit = func(account, ns, name string) (storer.Storer, error) {
		return open(ns, name)
	}
	rest := api.New(svc, cfg)
	git := githttp.New(open, tokenLookup{meta: mdb})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/git/") {
			git.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	}), nil
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
	if tok.State != types.TokenActive {
		return "", auth.ErrExpired
	}
	return tok.Scope, nil
}

// WriteUsage prints CLI help.
func WriteUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `artifacts — cloud-agnostic versioned git storage

Usage:
  artifacts serve
  artifacts migrate
  artifacts token create
`)
}
