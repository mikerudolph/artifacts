---
title: REST API reference
description: The implemented HTTP surface, request fields, response shapes, and pagination rules in one place.
---

## Base URL and conventions

Every route below is relative to:

```text
{origin}/client/v4/accounts/{account}/artifacts
```

For local development, use `http://127.0.0.1:8080` and account `local`. In `serve` mode send `Authorization: Bearer <control-token>`. JSON request bodies require `Content-Type: application/json`. All request fields are case-sensitive; unknown fields and trailing JSON values are rejected.

In the tables, `{ns}` means a namespace name and `{repo}` a repository name, not an ID. URL-encode path segments and query values. Credentials are the exception: revocation addresses a credential by its ID.

### JSON envelope

Successful JSON responses have `result`, `success: true`, `errors: []`, and `messages: []`. List endpoints may add `result_info`. Failures have `result: null`, `success: false`, and an `errors` array containing a numeric `code` and a `message`.

```json title="Example: a missing resource"
{
  "result": null,
  "success": false,
  "errors": [{"code": 10200, "message": "File not found"}],
  "messages": []
}
```

Check the HTTP status and `success`. `/file`, `/raw`, and `/blob` return **raw bytes on success**, not this envelope. Errors before streaming begins still use the JSON envelope. See [errors and limits](/artifacts/core/errors-and-limits/) for status handling.

Some empty collections currently serialize as `result: null` rather than `[]`. For list endpoints, normalize a successful null result to an empty collection; do not use a null result alone to detect failure.

## Namespaces

| Method | Route | Success |
| --- | --- | --- |
| `POST` | `/namespaces` | `200` · namespace object. |
| `GET` | `/namespaces` | `200` · namespace array with cursor `result_info`. |
| `GET` | `/namespaces/{ns}` | `200` · namespace object. |

Create body: `namespace` (required string), `jurisdiction` (optional `eu` or `us`). Namespace objects contain `namespace`, optional `jurisdiction`, `created_at`, and `updated_at`. Jurisdiction is metadata in this single-node implementation, not placement enforcement.

List query: `limit` (default 50, maximum 200) and `cursor`. There are no namespace update or delete routes. Repository creation implicitly ensures its namespace.

## Repositories

| Method | Route | Success |
| --- | --- | --- |
| `POST` | `/namespaces/{ns}/repos` | `200` · create result, including a secret token. |
| `GET` | `/namespaces/{ns}/repos` | `200` · repository array with cursor `result_info`. |
| `GET` | `/namespaces/{ns}/repos/{repo}` | `200` · repository object. |
| `PATCH` | `/namespaces/{ns}/repos/{repo}/settings` | `200` · updated repository object. |
| `DELETE` | `/namespaces/{ns}/repos/{repo}` | `202` · `{"id":"<repository-id>"}`. |

### Create and update fields

| Field | Create | Settings |
| --- | --- | --- |
| `name` | Required string. | Unsupported; no rename route. |
| `description` | Optional string. | Optional string, including empty to clear. |
| `default_branch` | Optional string; defaults to `main`. | Optional string; updates symbolic `HEAD`. |
| `read_only` | Optional boolean; defaults to `false`. | Optional boolean. |

The create result contains `id`, `name`, nullable `description`, `default_branch`, `remote`, and `token`. By default, `credential` also contains the initial credential's `id`, `plaintext`, `scope`, and `expires_at`; `token` is a compatibility plaintext alias. The credential grants 24-hour REST-content/Git write access. Set `issue_credential: false` to create without a secret (`credential` omitted, `token` empty). Creation returns an empty repository.

Creation supports `Idempotency-Key` only with `issue_credential: false`. Identical decoded input and key within the account/namespace returns the original create result; changed input returns `409` (`idempotency_conflict`). Successful results are retained indefinitely; replay does not recreate a deleted repository. Issue worker credentials separately. Publication keys are scoped separately to each repository. Fork/import and credential issuance do not support idempotent replay.

A repository read contains `id`, `name`, `description`, `default_branch`, `created_at`, `updated_at`, nullable `last_push_at`, `source`, `read_only`, `wal_sequence`, and `remote`. Optional `failure` and `deleted_at` fields exist in the record type, but failed and deleted repositories are hidden from normal lookup. The internal `status` field is **not** included in JSON.

Settings only change supplied fields. A missing default branch is not created by changing the setting. An initial control-token REST commit at WAL sequence zero is allowed even on a read-only repository; repository-credential writes and later REST publications respect read-only state.

### Repository list filters

| Query | Values / default |
| --- | --- |
| `search` | Repository name search string. |
| `sort` | `created_at` (default), `updated_at`, `last_push_at`, or `name`. |
| `direction` | `desc` (default) or `asc`. |
| `limit` | Default 50; maximum 200. |
| `cursor` | Opaque cursor from the previous response. |

Deletion tombstones the repository and revokes credentials before `202` is returned. Lookup then returns `404`. `result.id` is the repository ID, not a polling handle. The repository's jobs route is also inaccessible after deletion. Deletion retains history needed by forks. See [repository lifecycle](/artifacts/core/repositories/).

## Write a commit

**`POST /namespaces/{ns}/repos/{repo}/commits`** → `201`

```json title="Request body"
{
  "branch": "main",
  "message": "Publish the report",
  "author": {"name": "Worker", "email": "worker@example.com"},
  "files": [{"path": "report.md", "content": "# Report\nReady.\n"}]
}
```

`files` creates/updates selected paths; untouched paths are preserved. Optional `deletes` removes exact file paths. Accepts at most 100 combined changes, 1 MiB decoded string content, and 2 MiB JSON. At least one change is required unless `replace: true`, which explicitly replaces the whole tree and permits an empty file set. Replacement cannot also specify deletes.

`branch` defaults to the repository default; `message` defaults to `Initial artifacts`; author name/email default to `Artifacts Agent` / `agent@artifacts.local`. Optional `author.date` is an RFC 3339 timestamp and otherwise uses current time.

The JSON result is `{"sha":"<commit-sha>","sequence":1}`. Optional `expected_head` checks the full commit SHA; empty requires a nonexistent branch. New branches inherit the default branch unless `base` specifies a commit. `Idempotency-Key` safely replays successful publications. Repository write credentials can call this route. Merge and binary encodings are unsupported. See [write files](/artifacts/core/writing-files/) for details and migration from implicit snapshot replacement.

## Content and history

The following paths are relative to `/namespaces/{ns}/repos/{repo}`. All return HTTP `200` on success.

| Method | Suffix | Query / result |
| --- | --- | --- |
| `GET` | `/tree` | `ref` (default `HEAD`), `path` (default root). Result: `ref`, `commit`, `tree`, `path`, `entries`. |
| `GET` | `/file` | `ref` (default `HEAD`), `path` (file path). Raw `application/octet-stream` bytes. |
| `GET` | `/raw/{ref}/{path}` | Raw file bytes with detected content type. Prefer `/file` for refs with slashes. |
| `GET` | `/log` | `ref` (default `HEAD`), `limit` (default 20), `offset` (default 0). Array of log entries. |
| `GET` | `/commit/{hash}` | Commit SHA. Result: `hash`, `tree`, `parents`, `author`, `committer`, `message`. |
| `GET` | `/tree/{hash}` | Tree object SHA. Array of tree entries. |
| `GET` | `/blob/{hash}` | Blob object SHA. Raw `application/octet-stream` bytes. |
| `GET` | `/refs` | Array of `{"name":"refs/heads/main","sha":"<sha>"}` records. May include symbolic `HEAD`. |
| `GET` | `/events` | `after` (default 0), `limit` (1–100, default 100). Resumable committed publications; see below. |
| `GET` | `/wal` | Array of published immutable pack metadata. Storage diagnostics. |

A tree entry has `mode`, `type`, `hash`, and `name`. A log entry has `hash`, `message`, `author`, and `committer`. Signatures contain `name`, `email`, and `date`. WAL records contain `sequence`, `pack_key`, `index_key`, `checksum`, `size`, and `created_at`.

File/tree/log ref resolution supports `HEAD`, short branch or lightweight tag names, full refs, and full commit SHAs. It does not peel annotated tag objects or act as a general Git revision-expression parser. Pin a commit SHA for consistent multi-file reads. A new empty repository has no readable commit yet. See [read files and history](/artifacts/core/reading-files/).

## Publication events

`GET /namespaces/{ns}/repos/{repo}/events?after=0&limit=100` returns the usual envelope with this result:

```json
{
  "events": [{
    "repo_id": "repo_...",
    "sequence": 1,
    "created_at": "2026-09-19T12:00:00Z",
    "updates": [{"name": "refs/heads/main", "old_sha": "", "new_sha": "<commit-sha>"}]
  }],
  "next_after": 1
}
```

Events are ordered committed publications from REST, Git, imports, or legacy conversion. All ref changes in one publication stay together, even at a page boundary. Empty `old_sha` means ref creation; empty `new_sha` means deletion. A conversion can have no ref changes. Compaction emits no event. A snapshot fork starts its own sequence at zero and does not replay its parent's events.

Persist `next_after` **after** processing the page, then pass it as `after` on the next request. An empty page keeps the cursor unchanged. Repeated requests can return the same events: make consumers idempotent using `(repo_id, sequence)`, or include the ref name when processing individual ref changes. Events survive server restarts and compaction. They are retained with publication history; repository deletion makes the endpoint inaccessible.

Control tokens and repository read/write credentials can read the feed. Failed publications and idempotent retries produce no extra event. The API polls committed database records; it does not push webhooks or promise exactly-once execution in your application.

## Repository credentials

| Method | Route | Success |
| --- | --- | --- |
| `POST` | `/namespaces/{ns}/credentials` | `200` · `id`, `plaintext`, `scope`, `expires_at`. |
| `GET` | `/namespaces/{ns}/repos/{repo}/credentials` | `200` · credential metadata array with offset `result_info`. |
| `DELETE` | `/namespaces/{ns}/credentials/{id}` | `200` · `{"id":"<credential-id>"}`. |

Create body: `repo` (required repository name), `scope` (`read` or `write`, defaults to `write`), `ttl` (seconds, 60–31536000; omitted/zero defaults to 86400). Listing returns `id`, `scope`, `state`, `created_at`, and `expires_at`; it never returns plaintext or hashes.

List query: `state` (`active` by default, or `expired`, `revoked`, `all`), `page` (default 1), and `per_page` (default 30, maximum 100). The former `/tokens` routes are compatibility aliases with identical behavior. Use `/credentials` for new integrations.

These management routes require a control-plane token, even though the credentials they issue authorize both REST content and Git. See [authentication](/artifacts/core/authentication/).

## Forks, imports, and jobs

| Method | Route | Success |
| --- | --- | --- |
| `POST` | `/namespaces/{ns}/repos/{source}/fork` | `200` · create result for a new child. |
| `POST` | `/namespaces/{ns}/repos/{new-name}/import` | `200` · create result for the imported repository. |
| `GET` | `/namespaces/{ns}/repos/{repo}/jobs` | `200` · job array while the repository is reachable. |

Fork body: `name` (required new name), `description`, `read_only`, `default_branch_only` (optional, booleans default to false). The child stays in the same account and namespace, at the captured parent publication.

Import body: `url` (required public HTTPS URL), `branch`, `depth`, `read_only` (optional). The route creates the destination; do not pre-create it. No upstream credentials are supported. Imports and forks run synchronously within the HTTP request and also record durable jobs.

Jobs include `id`, `repo_id`, `kind`, `status`, optional `error`, `progress`, `created_at`, and `updated_at`. Kinds are `import`, `fork`, or `delete`; phases are `queued`, `running`, `succeeded`, or `failed`. There is no global job lookup, cancellation, or retry endpoint. See [forks and imports](/artifacts/core/forks-and-imports/) for behavior and failure recovery.

## Pagination shapes

Namespace and repository lists use cursors:

```json
{"cursor":"<next-page-cursor-or-empty>","per_page":50,"count":50}
```

Pass a non-empty returned cursor to the next request while keeping your filters consistent. An empty cursor means no next page. `count` is the number of results in the current page.

Credential lists use pages:

```json
{"page":1,"per_page":30,"total_pages":2,"count":30,"total_count":42}
```

Both objects appear at the top level as `result_info`. History uses `limit`/`offset` without this metadata. Refs, trees, WAL, and jobs have no pagination parameters; design for bounded repository sizes and avoid polling full diagnostic histories for routine application state.

## Implementation boundary

This reference describes the current checked-in service. It does not imply SDKs, webhooks, path-level permissions, object-store upload URLs, cross-account forks, REST merges, or physical purge endpoints. The [mounted routes](https://github.com/mikerudolph/artifacts/blob/main/internal/api/server.go) and [wire types](https://github.com/mikerudolph/artifacts/tree/main/internal/types) are the source of truth when extending the service.
