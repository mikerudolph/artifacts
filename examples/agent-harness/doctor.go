package main

import (
	"context"
	"net/http"
)

type jsonNamespace struct {
	Name string `json:"namespace"`
}

func doctor(ctx context.Context, h harness) (string, error) {
	_, err := callAPI[[]jsonNamespace](ctx, h, http.MethodGet, "/namespaces?limit=1", nil)
	if err != nil {
		return "", err
	}
	return redactURL(h.cfg.root), nil
}
