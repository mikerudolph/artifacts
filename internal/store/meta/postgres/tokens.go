package postgres

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s repoTokenStore) Create(ctx context.Context, tok types.RepoToken) (types.RepoToken, error) {
	if tok.ID == "" {
		tok.ID = types.TokenID(newID())
	}
	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = time.Now().UTC()
	}
	if tok.State == "" {
		tok.State = types.TokenActive
	}
	err := s.q.QueryRow(ctx, `
		INSERT INTO repo_tokens (id, repo_id, hash, scope, state, created_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, repo_id, hash, scope, state, created_at, expires_at`,
		string(tok.ID), string(tok.RepoID), tok.Hash, string(tok.Scope), string(tok.State), tok.CreatedAt, tok.ExpiresAt,
	).Scan(&tok.ID, &tok.RepoID, &tok.Hash, &tok.Scope, &tok.State, &tok.CreatedAt, &tok.ExpiresAt)
	if err != nil {
		return types.RepoToken{}, wrap(err)
	}
	return tok, nil
}

func (s repoTokenStore) GetByID(ctx context.Context, id types.TokenID) (types.RepoToken, error) {
	return s.scanToken(s.q.QueryRow(ctx, tokenSelect+` WHERE id = $1`, string(id)))
}

func (s repoTokenStore) GetByHash(ctx context.Context, hash string) (types.RepoToken, error) {
	return s.scanToken(s.q.QueryRow(ctx, tokenSelect+` WHERE hash = $1`, hash))
}

func (s repoTokenStore) List(ctx context.Context, repoID types.RepoID, state types.TokenState, page types.OffsetPage) ([]types.RepoToken, types.OffsetResult, error) {
	page = types.NormalizeOffsetPage(page)
	var total int
	countQ := `SELECT COUNT(*) FROM repo_tokens WHERE repo_id = $1 AND ($2 OR state = $3)`
	all := types.IsTokenStateAll(state)
	if err := s.q.QueryRow(ctx, countQ, string(repoID), all, string(state)).Scan(&total); err != nil {
		return nil, types.OffsetResult{}, err
	}
	offset := (page.Page - 1) * page.PerPage
	rows, err := s.q.Query(ctx, tokenSelect+`
		WHERE repo_id = $1 AND ($2 OR state = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`,
		string(repoID), all, string(state), page.PerPage, offset,
	)
	if err != nil {
		return nil, types.OffsetResult{}, err
	}
	defer rows.Close()
	var out []types.RepoToken
	for rows.Next() {
		tok, err := scanToken(rows)
		if err != nil {
			return nil, types.OffsetResult{}, err
		}
		out = append(out, tok)
	}
	pages := (total + page.PerPage - 1) / page.PerPage
	if pages == 0 {
		pages = 1
	}
	info := types.OffsetResult{Page: page.Page, PerPage: page.PerPage, TotalPages: pages, Count: len(out), TotalCount: total}
	return out, info, rows.Err()
}

func (s repoTokenStore) Revoke(ctx context.Context, id types.TokenID) error {
	tag, err := s.q.Exec(ctx, `UPDATE repo_tokens SET state = $2 WHERE id = $1`, string(id), string(types.TokenRevoked))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return meta.ErrNotFound
	}
	return nil
}

func (s apiTokenStore) Create(ctx context.Context, tok types.APIToken) (types.APIToken, error) {
	if tok.ID == "" {
		tok.ID = newID()
	}
	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = time.Now().UTC()
	}
	err := s.q.QueryRow(ctx, `
		INSERT INTO api_tokens (id, account_id, hash, created_at) VALUES ($1,$2,$3,$4)
		RETURNING id, account_id, hash, created_at`,
		tok.ID, string(tok.AccountID), tok.Hash, tok.CreatedAt,
	).Scan(&tok.ID, &tok.AccountID, &tok.Hash, &tok.CreatedAt)
	if err != nil {
		return types.APIToken{}, wrap(err)
	}
	return tok, nil
}

func (s apiTokenStore) GetByHash(ctx context.Context, hash string) (types.APIToken, error) {
	var tok types.APIToken
	err := s.q.QueryRow(ctx, `SELECT id, account_id, hash, created_at FROM api_tokens WHERE hash = $1`, hash).
		Scan(&tok.ID, &tok.AccountID, &tok.Hash, &tok.CreatedAt)
	if err != nil {
		return types.APIToken{}, wrap(err)
	}
	return tok, nil
}

var tokenSelect = "SELECT id, repo_id, " + "hash" + ", scope, state, created_at, expires_at FROM repo_tokens"

func (s repoTokenStore) scanToken(row rowScanner) (types.RepoToken, error) {
	tok, err := scanToken(row)
	if err != nil {
		return types.RepoToken{}, wrap(err)
	}
	return tok, nil
}

func scanToken(row rowScanner) (types.RepoToken, error) {
	var tok types.RepoToken
	err := row.Scan(&tok.ID, &tok.RepoID, &tok.Hash, &tok.Scope, &tok.State, &tok.CreatedAt, &tok.ExpiresAt)
	return tok, err
}
