package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

func (s namespaceStore) Create(ctx context.Context, ns types.Namespace) (types.Namespace, error) {
	if ns.ID == "" {
		ns.ID = types.NamespaceID(newID())
	}
	if ns.CreatedAt.IsZero() {
		ns.CreatedAt = time.Now().UTC()
	}
	ns.UpdatedAt = ns.CreatedAt
	err := s.q.QueryRow(ctx, `
		INSERT INTO namespaces (id, account_id, name, jurisdiction, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, account_id, name, jurisdiction, created_at, updated_at`,
		string(ns.ID), string(ns.AccountID), string(ns.Name), string(ns.Jurisdiction), ns.CreatedAt, ns.UpdatedAt,
	).Scan(&ns.ID, &ns.AccountID, &ns.Name, &ns.Jurisdiction, &ns.CreatedAt, &ns.UpdatedAt)
	if err != nil {
		return types.Namespace{}, wrap(err)
	}
	return ns, nil
}

func (s namespaceStore) GetByName(ctx context.Context, accountID types.AccountID, name types.NamespaceName) (types.Namespace, error) {
	var ns types.Namespace
	err := s.q.QueryRow(ctx, `
		SELECT id, account_id, name, jurisdiction, created_at, updated_at
		FROM namespaces WHERE account_id = $1 AND name = $2`,
		string(accountID), string(name),
	).Scan(&ns.ID, &ns.AccountID, &ns.Name, &ns.Jurisdiction, &ns.CreatedAt, &ns.UpdatedAt)
	if err != nil {
		return types.Namespace{}, wrap(err)
	}
	return ns, nil
}

func (s namespaceStore) List(ctx context.Context, accountID types.AccountID, page types.CursorPage) ([]types.Namespace, types.CursorResult, error) {
	page = types.NormalizeCursorPage(page)
	curTime, curID, err := decodeCursor(page.Cursor)
	if err != nil {
		return nil, types.CursorResult{}, err
	}
	rows, err := s.q.Query(ctx, `
		SELECT id, account_id, name, jurisdiction, created_at, updated_at
		FROM namespaces
		WHERE account_id = $1
		  AND ($2 = '' OR (created_at, id) < ($3::timestamptz, $2))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`,
		string(accountID), curID, curTime, page.Limit+1,
	)
	if err != nil {
		return nil, types.CursorResult{}, err
	}
	defer rows.Close()
	var out []types.Namespace
	for rows.Next() {
		var ns types.Namespace
		if err := rows.Scan(&ns.ID, &ns.AccountID, &ns.Name, &ns.Jurisdiction, &ns.CreatedAt, &ns.UpdatedAt); err != nil {
			return nil, types.CursorResult{}, err
		}
		out = append(out, ns)
	}
	info := types.CursorResult{PerPage: page.Limit, Count: len(out)}
	if len(out) > page.Limit {
		out = out[:page.Limit]
		info.Count = page.Limit
		last := out[len(out)-1]
		info.Cursor = encodeCursor(last.CreatedAt, string(last.ID))
	}
	return out, info, rows.Err()
}

func encodeCursor(t time.Time, id string) string {
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cur string) (time.Time, string, error) {
	if cur == "" {
		return time.Now().UTC(), "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(cur)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	return ts, parts[1], nil
}
