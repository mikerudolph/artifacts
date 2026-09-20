package postgres

import (
	"context"
	"testing"

	"github.com/mikerudolph/artifacts/internal/testkit"
)

func TestDeploymentSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dsn := testkit.Postgres(t)
	metadata, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	s := metadata.(*store)
	defer s.Close()
	if s.CheckSchema(ctx) == nil {
		t.Fatal("accepted missing schema")
	}
	if err := Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"UPDATE schema_migrations SET dirty=true", "UPDATE schema_migrations SET dirty=false, version=0", "UPDATE schema_migrations SET version=999999"} {
		if _, err := s.q.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
		if s.CheckSchema(ctx) == nil {
			t.Fatal("accepted incompatible schema")
		}
	}
}
