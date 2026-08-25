package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

var errTokenWrite = errors.New("token write failed")

type failingTokenMeta struct{ meta.Store }

func (m failingTokenMeta) RepoTokens() meta.RepoTokens {
	return failingRepoTokens{RepoTokens: m.Store.RepoTokens()}
}

type failingRepoTokens struct{ meta.RepoTokens }

func (failingRepoTokens) Create(context.Context, types.RepoToken) (types.RepoToken, error) {
	return types.RepoToken{}, errTokenWrite
}

func TestImportRecordsCredentialFailure(t *testing.T) {
	runner := testRunner(t)
	runner.meta = failingTokenMeta{Store: runner.meta}
	runner.imports = successfulImport{}
	_, err := runner.Import(context.Background(), "local", "default", "token-failure", types.ImportRepoInput{
		URL: "https://93.184.216.34/repo.git",
	})
	if !errors.Is(err, errTokenWrite) {
		t.Fatalf("import error %v", err)
	}
}
