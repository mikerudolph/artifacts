package repository

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestCommitLockCancellationAndSourceFailure(t *testing.T) {
	manager, err := New(newMemoryMeta(), objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "repo", AccountID: "account", DefaultBranch: "main", Status: types.RepoReady}
	_, unlock, err := manager.lockedPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := manager.Commit(ctx, repo, types.CommitInput{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cache wait: %v", err)
	}
	failure := errors.New("source unavailable")
	file := types.CommitFile{Source: &types.CommitSource{Open: func() (io.ReadCloser, error) { return nil, failure }}}
	if _, err := hashCommitFile(t.Context(), "unused", file); !errors.Is(err, failure) {
		t.Fatalf("source failure: %v", err)
	}
}

func TestMultipartDigestSemantics(t *testing.T) {
	input := types.CommitInput{IdempotencyKey: "operation", Multipart: true, Files: []types.CommitFile{
		{Path: "b", Source: &types.CommitSource{Size: 1, SHA256: strings.Repeat("a", 64)}},
		{Path: "a", Mode: "100755", Source: &types.CommitSource{Size: 0, SHA256: strings.Repeat("b", 64)}},
	}, Deletes: []string{"z", "y"}}
	original, err := commitDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Files[0], input.Files[1] = input.Files[1], input.Files[0]
	input.Deletes[0], input.Deletes[1] = input.Deletes[1], input.Deletes[0]
	reordered, err := commitDigest(input)
	if err != nil || original != reordered {
		t.Fatal("order changed multipart digest")
	}
	input.Files[0].Mode = "100644"
	changed, _ := commitDigest(input)
	if changed == original {
		t.Fatal("mode omitted from digest")
	}
	input.ExpectedHead = new(string)
	head, _ := commitDigest(input)
	if changed == head {
		t.Fatal("absent and empty expected heads conflated")
	}
	input.Files[0].Source = nil
	if _, err := commitDigest(input); err == nil {
		t.Fatal("missing source accepted")
	}
	input.IdempotencyKey = " invalid"
	if _, err := commitDigest(input); err == nil {
		t.Fatal("invalid key accepted")
	}
}
