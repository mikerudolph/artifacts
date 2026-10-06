package repository

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func commitDigest(input types.CommitInput) (string, error) {
	if input.IdempotencyKey == "" {
		return "", nil
	}
	if err := meta.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return "", err
	}
	if !input.Multipart {
		return meta.RequestDigest(input)
	}
	type fileDigest struct {
		Path, Mode, SHA256 string
		Size               int64
	}
	files := make([]fileDigest, 0, len(input.Files))
	for _, file := range input.Files {
		if file.Source == nil {
			return "", &types.InputError{Field: "/files", Message: "multipart content source missing"}
		}
		files = append(files, fileDigest{Path: file.Path, Mode: file.Mode, SHA256: file.Source.SHA256, Size: file.Source.Size})
	}
	slices.SortFunc(files, func(a, b fileDigest) int { return strings.Compare(a.Path, b.Path) })
	input.Files = nil
	input.Deletes = slices.Clone(input.Deletes)
	slices.Sort(input.Deletes)
	digest, err := meta.RequestDigest(struct {
		Version string
		Input   types.CommitInput
		Files   []fileDigest
	}{"multipart-v1", input, files})
	return digest, err
}

func commitScope(repo types.Repo) string {
	return "commit/" + string(repo.AccountID) + "/" + string(repo.ID)
}

func authorizeCommit(ctx context.Context, input types.CommitInput) error {
	if input.Authorize != nil {
		return input.Authorize(ctx)
	}
	return ctx.Err()
}

func (m *Manager) replayCommit(ctx context.Context, repo types.Repo, input types.CommitInput, digest string) (types.CommitResult, bool, error) {
	var result types.CommitResult
	if err := authorizeCommit(ctx, input); err != nil {
		return result, false, err
	}
	if input.IdempotencyKey == "" {
		return result, false, nil
	}
	reader, ok := m.meta.(meta.IdempotentReader)
	if !ok {
		return result, false, nil
	}
	body, err := reader.FindIdempotent(ctx, commitScope(repo), input.IdempotencyKey, digest)
	if meta.IsNotFound(err) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	err = json.Unmarshal(body, &result)
	return result, err == nil, err
}

func (m *Manager) publishCommit(ctx context.Context, repo types.Repo, input types.CommitInput, candidate preparedCommit, digest string) (types.CommitResult, error) {
	if err := authorizeCommit(ctx, input); err != nil {
		return types.CommitResult{}, err
	}
	publish := func(st meta.V2Store) (types.CommitResult, error) {
		sequence, err := st.WAL().Publish(ctx, candidate.publication)
		return types.CommitResult{SHA: candidate.sha, Sequence: sequence}, err
	}
	if input.IdempotencyKey == "" {
		return publish(m.meta)
	}
	return meta.IdempotentDigest(ctx, m.meta, commitScope(repo), input.IdempotencyKey, digest, func(tx meta.Store) (types.CommitResult, error) {
		return publish(tx.(meta.V2Store))
	})
}
