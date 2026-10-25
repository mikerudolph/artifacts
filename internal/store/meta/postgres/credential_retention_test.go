package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func retentionStore(t *testing.T) (*store, types.Repo, string) {
	t.Helper()
	dsn := testkit.Postgres(t)
	if err := Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	metadata, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	s := metadata.(*store)
	t.Cleanup(s.Close)
	_, repo := seedGraph(t, s, "local")
	return s, repo, dsn
}

func retentionExec(t *testing.T, s *store, query string, args ...any) {
	t.Helper()
	if _, err := s.q.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialRetentionBoundaries(t *testing.T) {
	s, repo, _ := retentionStore(t)
	cutoff := time.Now().UTC().Truncate(time.Second).Add(-7 * 24 * time.Hour)
	for _, tc := range []struct {
		id      string
		expiry  time.Time
		revoked *time.Time
		removed bool
	}{
		{"expired", cutoff.Add(-time.Second), nil, true},
		{"boundary", cutoff, nil, false},
		{"recent", cutoff.Add(time.Second), nil, false},
		{"valid", cutoff.Add(8 * 24 * time.Hour), nil, false},
		{"revoked", cutoff.Add(8 * 24 * time.Hour), timePointer(cutoff.Add(-time.Second)), true},
		{"revoked-boundary", cutoff.Add(time.Hour), &cutoff, false},
		{"expired-then-revoked", cutoff.Add(-time.Hour), timePointer(cutoff.Add(time.Hour)), true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			state := "active"
			if tc.revoked != nil {
				state = "revoked"
			}
			retentionExec(t, s, `INSERT INTO repo_tokens(id,repo_id,hash,scope,state,expires_at,revoked_at)
				VALUES($1,$2,$1,'write',$3,$4,$5)`, tc.id, repo.ID, state, tc.expiry, tc.revoked)
			count, err := s.RepoTokens().Prune(t.Context(), cutoff)
			if err != nil || (count == 1) != tc.removed {
				t.Fatalf("pruned %d: %v", count, err)
			}
			_, err = s.RepoTokens().GetByID(t.Context(), types.TokenID(tc.id))
			if meta.IsNotFound(err) != tc.removed {
				t.Fatalf("credential lookup: %v", err)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time { return &value }

func TestRevocationPreservesFirstInvalidation(t *testing.T) {
	s, repo, _ := retentionStore(t)
	for _, state := range []types.TokenState{types.TokenActive, types.TokenRevoked} {
		tok, err := s.RepoTokens().Create(t.Context(), types.RepoToken{
			RepoID: repo.ID, Hash: string(state), State: state, Scope: types.ScopeRead, ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.RepoTokens().Revoke(t.Context(), tok.ID); err != nil {
			t.Fatal(err)
		}
		first := revokedAt(t, s, tok.ID)
		for range 2 {
			if err := s.RepoTokens().Revoke(t.Context(), tok.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.RepoTokens().RevokeAll(t.Context(), repo.ID); err != nil {
				t.Fatal(err)
			}
		}
		if !first.Equal(revokedAt(t, s, tok.ID)) {
			t.Fatal("repeated revocation extended retention")
		}
	}
	if _, err := s.q.Exec(t.Context(), `UPDATE repo_tokens SET revoked_at=NULL WHERE state='revoked'`); err == nil {
		t.Fatal("revoked credential lost invalidation timestamp")
	}
}

func revokedAt(t *testing.T, s *store, id types.TokenID) time.Time {
	t.Helper()
	var at time.Time
	if err := s.q.QueryRow(t.Context(), "SELECT revoked_at FROM repo_tokens WHERE id=$1", id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func seedExpiredCredentials(t *testing.T, s *store, repo types.RepoID, count int) {
	t.Helper()
	retentionExec(t, s, `INSERT INTO repo_tokens(id,repo_id,hash,scope,state,expires_at)
		SELECT 'expired-'||i,$1,'hash-'||i,'read','active',now()-interval '10 days' FROM generate_series(0,$2-1) i`, repo, count)
}

func TestCredentialPruningConcurrentBatchesAndLockedRows(t *testing.T) {
	s, repo, dsn := retentionStore(t)
	seedExpiredCredentials(t, s, repo.ID, 2405)
	other, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer other.(*store).Close()
	locked, err := s.p.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locked.Rollback(context.Background()) }()
	if _, err := locked.Exec(t.Context(), "SELECT id FROM repo_tokens WHERE id='expired-0' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	var total atomic.Int64
	var workers sync.WaitGroup
	errs := make(chan error, 2)
	for _, source := range []meta.Store{s, other} {
		workers.Go(func() {
			errs <- drainCredentials(t.Context(), source.RepoTokens(), &total)
		})
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if total.Load() != 2404 {
		t.Fatalf("deleted %d rows", total.Load())
	}
	if err := locked.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RepoTokens().Prune(t.Context(), time.Now()); n != 1 || err != nil {
		t.Fatalf("unlocked row: %d %v", n, err)
	}
}

func drainCredentials(ctx context.Context, tokens meta.RepoTokens, total *atomic.Int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		count, err := tokens.Prune(ctx, time.Now().Add(-7*24*time.Hour))
		if err != nil || count == 0 {
			return err
		}
		if count > 1000 {
			return fmt.Errorf("unbounded cleanup: %d", count)
		}
		total.Add(count)
	}
}

func TestCredentialPruningRollbackAndCancellation(t *testing.T) {
	s, repo, _ := retentionStore(t)
	seedExpiredCredentials(t, s, repo.ID, 2)
	rollback := errors.New("abort cleanup")
	if err := s.RunInTx(t.Context(), func(tx meta.Store) error {
		if n, err := tx.RepoTokens().Prune(t.Context(), time.Now()); n != 2 || err != nil {
			t.Fatalf("transactional cleanup: %d %v", n, err)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.RepoTokens().Prune(ctx, time.Now()); err == nil {
		t.Fatal("canceled cleanup succeeded")
	}
	if n, err := s.RepoTokens().Prune(t.Context(), time.Now()); n != 2 || err != nil {
		t.Fatalf("retry cleanup: %d %v", n, err)
	}
}

func TestCredentialPruningCancelsBlockedStatement(t *testing.T) {
	s, repo, _ := retentionStore(t)
	seedExpiredCredentials(t, s, repo.ID, 1)
	locked, err := s.p.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locked.Rollback(context.Background()) }()
	if _, err := locked.Exec(t.Context(), "LOCK TABLE repo_tokens IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := s.RepoTokens().Prune(ctx, time.Now()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked cleanup did not honor deadline: %v", err)
	}
	if err := locked.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RepoTokens().Prune(t.Context(), time.Now()); n != 1 || err != nil {
		t.Fatalf("retry blocked cleanup: %d %v", n, err)
	}
}
