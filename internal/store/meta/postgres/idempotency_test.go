package postgres

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestIdempotencyRollbackAndScope(t *testing.T) {
	ctx, st, ns := v2Fixture(t)
	store := st.(meta.IdempotentStore)
	boom := errors.New("response encoding failed")
	_, err := store.RunIdempotent(ctx, "scope", "key", "digest", func(tx meta.Store) (json.RawMessage, error) {
		_, err := tx.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "rollback"})
		if err != nil {
			return nil, err
		}
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := st.Repos().GetByName(ctx, ns.ID, "rollback"); !meta.IsNotFound(err) {
		t.Fatal("failed mutation was not rolled back")
	}
	calls := 0
	create := func(tx meta.Store) (json.RawMessage, error) {
		calls++
		repo, err := tx.Repos().Create(ctx, types.Repo{NamespaceID: ns.ID, Name: "rollback"})
		if err != nil {
			return nil, err
		}
		return json.Marshal(repo.ID)
	}
	first, err := store.RunIdempotent(ctx, "scope", "key", "digest", create)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.RunIdempotent(ctx, "scope", "key", "digest", create)
	if err != nil || string(first) != string(again) || calls != 1 {
		t.Fatalf("replay %s %v calls=%d", again, err, calls)
	}
	if _, err := store.RunIdempotent(ctx, "scope", "key", "different", create); !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	_, err = store.RunIdempotent(ctx, "other-tenant", "key", "different", func(meta.Store) (json.RawMessage, error) { return json.RawMessage(`"separate"`), nil })
	if err != nil {
		t.Fatal(err)
	}
}
