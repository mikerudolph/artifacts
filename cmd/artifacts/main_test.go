package main

import (
	"context"
	"os"
	"testing"

	"github.com/mikerudolph/artifacts/internal/app"
)

func TestMainHelp(t *testing.T) {
	t.Parallel()
	if code := app.Run(context.Background(), nil, os.Stdout, os.Stderr); code != 0 {
		t.Fatalf("code %d", code)
	}
}
