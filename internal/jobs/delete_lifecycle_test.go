package jobs

import (
	"context"
	"sync"
	"testing"

	"github.com/mikerudolph/artifacts/internal/types"
)

func TestDeleteIsIdempotentAndRevokesCredentials(t *testing.T) {
	runner := testRunner(t)
	ctx := context.Background()
	runner.imports = successfulImport{}
	created, err := runner.Import(ctx, "local", "default", "delete-ready", types.ImportRepoInput{
		URL: "https://93.184.216.34/repo.git",
	})
	if err != nil {
		t.Fatal(err)
	}
	deleteConcurrently(t, ctx, runner)
	repo, err := runner.meta.Repos().GetByID(ctx, created.ID)
	if err != nil || repo.Status != types.RepoDeleted || repo.DeletedAt == nil {
		t.Fatalf("repo %+v %v", repo, err)
	}
	tokens, _, err := runner.meta.RepoTokens().List(ctx, repo.ID, types.TokenState("all"), types.OffsetPage{})
	if err != nil || len(tokens) != 1 || tokens[0].State != types.TokenRevoked {
		t.Fatalf("tokens %+v %v", tokens, err)
	}
	jobs, err := runner.List(ctx, repo.ID)
	if err != nil || countDeleteJobs(jobs) != 1 {
		t.Fatalf("jobs %+v %v", jobs, err)
	}
	if _, err := runner.DeleteRepo(ctx, "local", "default", "delete-ready"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	stale := repo
	stale.Status = types.RepoReady
	if _, err := runner.meta.Repos().Update(ctx, stale); err == nil {
		t.Fatal("stale update resurrected deleted repository")
	}
	repo, err = runner.meta.Repos().GetByID(ctx, created.ID)
	if err != nil || repo.Status != types.RepoDeleted {
		t.Fatalf("resurrected repo %+v %v", repo, err)
	}
}

func TestCredentialIssueDeleteRaceLeavesNoActiveToken(t *testing.T) {
	runner := testRunner(t)
	runner.imports = successfulImport{}
	ctx := context.Background()
	created, err := runner.Import(ctx, "local", "default", "token-race", types.ImportRepoInput{URL: "https://93.184.216.34/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = runner.mint(ctx, created.ID)
		}()
	}
	close(start)
	if _, err := runner.DeleteRepo(ctx, "local", "default", "token-race"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	tokens, _, err := runner.meta.RepoTokens().List(ctx, created.ID, types.TokenState("all"), types.OffsetPage{PerPage: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if token.State == types.TokenActive {
			t.Fatalf("active token survived delete race: %s", token.ID)
		}
	}
}

func deleteConcurrently(t *testing.T, ctx context.Context, runner *Runner) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := runner.DeleteRepo(ctx, "local", "default", "delete-ready")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("delete race: %v", err)
		}
	}
}

func countDeleteJobs(jobs []types.Job) int {
	count := 0
	for _, job := range jobs {
		if job.Kind == types.JobDelete {
			count++
		}
	}
	return count
}
