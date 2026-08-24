package service

import (
	"context"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestForkImportWrappers(t *testing.T) {
	t.Parallel()
	svc := New(newFake(), nil, "http://x")
	ctx := context.Background()
	_, err := svc.CreateRepo(ctx, "local", "default", types.CreateRepoInput{Name: "src"})
	if err != nil {
		t.Fatal(err)
	}
	objs := objecttest.NewMem()
	if _, err := svc.Fork(ctx, objs, "local", "default", "src", types.ForkRepoInput{Name: "dst"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Import(ctx, objs, "local", "default", "bad", types.ImportRepoInput{}); err == nil {
		t.Fatal("expected invalid url")
	}
}
