package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s repoStore) Create(ctx context.Context, repo types.Repo) (types.Repo, error) {
	var namespaceAccount types.AccountID
	if err := s.q.QueryRow(ctx, `SELECT account_id FROM namespaces WHERE id=$1`, repo.NamespaceID).Scan(&namespaceAccount); err != nil {
		return types.Repo{}, wrap(err)
	}
	if repo.AccountID != "" && repo.AccountID != namespaceAccount {
		return types.Repo{}, meta.ErrNotFound
	}
	repo.AccountID = namespaceAccount
	if repo.ID == "" {
		repo.ID = newRepoID()
	}
	if repo.DefaultBranch == "" {
		repo.DefaultBranch = types.DefaultBranch
	}
	if repo.Status == "" {
		repo.Status = types.RepoReady
	}
	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = time.Now().UTC()
	}
	repo.UpdatedAt = repo.CreatedAt
	if repo.StorageVersion == 0 {
		repo.StorageVersion = 2
	}
	err := s.q.QueryRow(ctx, `
		INSERT INTO repos (id, namespace_id, account_id, name, description, default_branch, read_only, source, status,
			storage_version, wal_sequence, failure, deleted_at, created_at, updated_at, last_push_at)
		VALUES ($1,$2,COALESCE(NULLIF($3,''),(SELECT account_id FROM namespaces WHERE id=$2)),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id, namespace_id, account_id, name, description, default_branch, read_only, source, status,
			storage_version, wal_sequence, failure, deleted_at, created_at, updated_at, last_push_at`,
		string(repo.ID), string(repo.NamespaceID), string(repo.AccountID), string(repo.Name), repo.Description, repo.DefaultBranch,
		repo.ReadOnly, repo.Source, string(repo.Status), repo.StorageVersion, repo.WALSequence, repo.Failure,
		repo.DeletedAt, repo.CreatedAt, repo.UpdatedAt, repo.LastPushAt,
	).Scan(&repo.ID, &repo.NamespaceID, &repo.AccountID, &repo.Name, &repo.Description, &repo.DefaultBranch,
		&repo.ReadOnly, &repo.Source, &repo.Status, &repo.StorageVersion, &repo.WALSequence,
		&repo.Failure, &repo.DeletedAt, &repo.CreatedAt, &repo.UpdatedAt, &repo.LastPushAt)
	if err != nil {
		return types.Repo{}, wrap(err)
	}
	return repo, nil
}

func (s repoStore) GetByName(ctx context.Context, namespaceID types.NamespaceID, name types.RepoName) (types.Repo, error) {
	return s.scanRepo(s.q.QueryRow(ctx, repoSelect+` WHERE namespace_id = $1 AND name = $2`, string(namespaceID), string(name)))
}

func (s repoStore) GetByID(ctx context.Context, id types.RepoID) (types.Repo, error) {
	return s.scanRepo(s.q.QueryRow(ctx, repoSelect+` WHERE id = $1`, string(id)))
}

func (s repoStore) Update(ctx context.Context, repo types.Repo) (types.Repo, error) {
	repo.UpdatedAt = time.Now().UTC()
	err := s.q.QueryRow(ctx, `
		UPDATE repos SET description=$2, default_branch=$3, read_only=$4, source=$5, status=$6,
			updated_at=$7, last_push_at=$8, failure=$9, deleted_at=$10
		WHERE id=$1 AND NOT (status IN ('deleting','deleted') AND $6 NOT IN ('deleting','deleted'))
		RETURNING id, namespace_id, account_id, name, description, default_branch, read_only, source, status,
			storage_version, wal_sequence, failure, deleted_at, created_at, updated_at, last_push_at`,
		string(repo.ID), repo.Description, repo.DefaultBranch, repo.ReadOnly, repo.Source, string(repo.Status), repo.UpdatedAt, repo.LastPushAt,
		repo.Failure, repo.DeletedAt,
	).Scan(&repo.ID, &repo.NamespaceID, &repo.AccountID, &repo.Name, &repo.Description, &repo.DefaultBranch,
		&repo.ReadOnly, &repo.Source, &repo.Status, &repo.StorageVersion, &repo.WALSequence,
		&repo.Failure, &repo.DeletedAt, &repo.CreatedAt, &repo.UpdatedAt, &repo.LastPushAt)
	if err != nil {
		if meta.IsNotFound(wrap(err)) {
			return types.Repo{}, meta.ErrCASConflict
		}
		return types.Repo{}, wrap(err)
	}
	return repo, nil
}

func (s repoStore) Transition(ctx context.Context, id types.RepoID, from, to types.RepoStatus, deletedAt *time.Time) (types.Repo, error) {
	var repo types.Repo
	err := s.q.QueryRow(ctx, `UPDATE repos SET status=$3,deleted_at=$4,updated_at=now() WHERE id=$1 AND status=$2
		RETURNING id, namespace_id, account_id, name, description, default_branch, read_only, source, status,
		storage_version, wal_sequence, failure, deleted_at, created_at, updated_at, last_push_at`,
		id, from, to, deletedAt).Scan(&repo.ID, &repo.NamespaceID, &repo.AccountID, &repo.Name, &repo.Description,
		&repo.DefaultBranch, &repo.ReadOnly, &repo.Source, &repo.Status, &repo.StorageVersion, &repo.WALSequence,
		&repo.Failure, &repo.DeletedAt, &repo.CreatedAt, &repo.UpdatedAt, &repo.LastPushAt)
	if meta.IsNotFound(wrap(err)) {
		return types.Repo{}, meta.ErrCASConflict
	}
	return repo, err
}

func (s repoStore) Delete(ctx context.Context, id types.RepoID) error {
	tag, err := s.q.Exec(ctx, `DELETE FROM repos WHERE id = $1`, string(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return meta.ErrNotFound
	}
	return nil
}

func (s repoStore) List(ctx context.Context, opts meta.ListReposOpts) ([]types.Repo, types.CursorResult, error) {
	opts.Page = types.NormalizeCursorPage(opts.Page)
	order, ok := repoOrder(opts.Sort, opts.Direction)
	if !ok {
		return nil, types.CursorResult{}, types.ErrInvalidSort
	}
	search := "%" + opts.Search + "%"
	offset, err := decodeRepoCursor(opts.Page.Cursor)
	if err != nil {
		return nil, types.CursorResult{}, err
	}
	q := repoSelect + ` WHERE namespace_id = $1 AND status <> 'deleted' AND ($2 = '%%' OR name ILIKE $2) ORDER BY ` + order + ` LIMIT $3 OFFSET $4`
	rows, err := s.q.Query(ctx, q, string(opts.NamespaceID), search, opts.Page.Limit+1, offset)
	if err != nil {
		return nil, types.CursorResult{}, err
	}
	defer rows.Close()
	var out []types.Repo
	for rows.Next() {
		repo, err := scanRepo(rows)
		if err != nil {
			return nil, types.CursorResult{}, err
		}
		out = append(out, repo)
	}
	info := types.CursorResult{PerPage: opts.Page.Limit, Count: len(out)}
	if len(out) > opts.Page.Limit {
		out = out[:opts.Page.Limit]
		info.Count = opts.Page.Limit
		info.Cursor = encodeRepoCursor(offset + opts.Page.Limit)
	}
	return out, info, rows.Err()
}

func encodeRepoCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("offset:" + strconv.Itoa(offset)))
}

func decodeRepoCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(raw), "offset:") {
		return 0, fmt.Errorf("invalid cursor")
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(string(raw), "offset:"))
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return offset, nil
}

const repoSelect = `SELECT id, namespace_id, account_id, name, description, default_branch, read_only, source, status,
	storage_version, wal_sequence, failure, deleted_at, created_at, updated_at, last_push_at FROM repos`

type rowScanner interface {
	Scan(dest ...any) error
}

func (s repoStore) scanRepo(row rowScanner) (types.Repo, error) {
	repo, err := scanRepo(row)
	if err != nil {
		return types.Repo{}, wrap(err)
	}
	return repo, nil
}

func scanRepo(row rowScanner) (types.Repo, error) {
	var repo types.Repo
	err := row.Scan(&repo.ID, &repo.NamespaceID, &repo.AccountID, &repo.Name, &repo.Description, &repo.DefaultBranch,
		&repo.ReadOnly, &repo.Source, &repo.Status, &repo.StorageVersion, &repo.WALSequence,
		&repo.Failure, &repo.DeletedAt, &repo.CreatedAt, &repo.UpdatedAt, &repo.LastPushAt)
	return repo, err
}

func repoOrder(sort types.RepoSortField, dir types.SortDirection) (string, bool) {
	field := string(sort)
	if field == "" {
		field = string(types.SortCreatedAt)
	}
	switch types.RepoSortField(field) {
	case types.SortCreatedAt, types.SortUpdatedAt, types.SortLastPushAt, types.SortName:
	default:
		return "", false
	}
	d := "DESC"
	if dir == types.SortAsc {
		d = "ASC"
	}
	return fmt.Sprintf("%s %s, id %s", field, d, d), true
}
