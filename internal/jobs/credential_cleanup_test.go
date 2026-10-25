package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestDeleteWithCredentialCleanupAndMinting(t *testing.T) {
	runner := testRunner(t)
	runner.imports = successfulImport{}
	created, err := runner.Import(t.Context(), "local", "default", "cleanup-delete", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 250 {
		_, err := runner.meta.RepoTokens().Create(t.Context(), types.RepoToken{
			RepoID: created.ID, Hash: fmt.Sprintf("credential-%d", i), Scope: types.ScopeWrite,
			ExpiresAt: time.Now().Add(time.Duration(i-125) * 24 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() { _, _ = runner.mint(ctx, created.ID) })
	}
	workers.Go(func() {
		for range 20 {
			if _, err := runner.meta.RepoTokens().Prune(ctx, time.Now().Add(-7*24*time.Hour)); err != nil && ctx.Err() == nil {
				t.Error("cleanup during deletion:", err)
				return
			}
		}
	})
	_, err = runner.DeleteRepo(t.Context(), "local", "default", "cleanup-delete")
	workers.Wait()
	if err != nil {
		t.Fatal(err)
	}
	_, page, err := runner.meta.RepoTokens().List(t.Context(), created.ID, types.TokenActive, types.OffsetPage{})
	if err != nil || page.TotalCount != 0 {
		t.Fatalf("active credentials survived: %+v %v", page, err)
	}
	if _, err := runner.mint(t.Context(), created.ID); err == nil {
		t.Fatal("minted credential after deletion")
	}
}

type revocationFailure struct{ meta.Store }

func (m revocationFailure) RepoTokens() meta.RepoTokens {
	return revocationFailureTokens{m.Store.RepoTokens()}
}

func (m revocationFailure) RunInTx(ctx context.Context, visit func(meta.Store) error) error {
	return m.Store.RunInTx(ctx, func(tx meta.Store) error { return visit(revocationFailure{tx}) })
}

type revocationFailureTokens struct{ meta.RepoTokens }

func (revocationFailureTokens) RevokeAll(context.Context, types.RepoID) error {
	return errTokenWrite
}

func TestDeleteRollsBackWhenCredentialRevocationFails(t *testing.T) {
	runner := testRunner(t)
	runner.imports = successfulImport{}
	created, err := runner.Import(t.Context(), "local", "default", "revoke-failure", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	original := runner.meta
	runner.meta = revocationFailure{original}
	if _, err := runner.DeleteRepo(t.Context(), "local", "default", "revoke-failure"); !errors.Is(err, errTokenWrite) {
		t.Fatal("revocation failure lost:", err)
	}
	repo, err := original.Repos().GetByID(t.Context(), created.ID)
	if err != nil || repo.Status != types.RepoReady {
		t.Fatal("failed revocation left repository deleting:", err)
	}
	jobs, err := original.Jobs().ListByRepo(t.Context(), created.ID)
	if err != nil || countDeleteJobs(jobs) != 0 {
		t.Fatal("failed deletion left a delete job:", err)
	}
	runner.meta = original
	if _, err := runner.DeleteRepo(t.Context(), "local", "default", "revoke-failure"); err != nil {
		t.Fatal("delete retry failed:", err)
	}
}
