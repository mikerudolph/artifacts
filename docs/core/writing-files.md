---
title: Write files
description: Update selected files atomically through REST, with version checks and safe retries.
---

REST and Git write the same repository history. REST handles bounded text changes; Git handles binary content, larger changes, file modes, and merges.

## Update selected files

`POST /commits` updates the supplied `files` and preserves every untouched path. Use `deletes` to remove named files. All changes publish together in one Git commit.

```json
{
  "branch": "main",
  "expected_head": "0123456789abcdef0123456789abcdef01234567",
  "message": "Complete the report",
  "files": [{"path": "outputs/report.md", "content": "# Report\nReady.\n"}],
  "deletes": ["outputs/draft.md"]
}
```

Use an actual SHA from `/tree` or `/refs` for `expected_head`. An untouched `inputs/brief.md` remains in the new version, including when the existing repository contains binary files or executable files. Updating an existing executable preserves its executable mode. Updating a symlink path replaces that entry with a regular file; paths beneath a symlink are rejected.

Send this body to `$REPO_API/commits` with an account control token or a repository write credential. Successful publication returns HTTP `201` and `{"sha":"<commit-sha>","sequence":1}` inside `result`. Use `sha` for reproducible reads; `sequence` is storage publication metadata.

## Coordinate writers

`expected_head` is an optional precondition. Supply the full lowercase SHA you read; a changed head returns HTTP `409`, `errors[0].kind: "head_conflict"`, and `errors[0].current_head`. Nothing from that request publishes. Fetch the new version and reconcile the changes before trying again. This detects intervening Git pushes as well as REST writes.

Use `expected_head: ""` to require a branch that does not yet exist. Omitting the field applies changes against the current head at execution time; it does **not** detect a stale application read. Applications that derive changes from existing files should always provide it.

A new branch inherits the current default branch's commit when available. To branch from a specific version, supply `base` with a full commit SHA and `expected_head: ""`. `base` is only valid for a new branch and must identify a commit present in the repository. In an empty repository, the first write creates a root commit.

## Delete or replace explicitly

`deletes` contains exact file paths, not recursive directory prefixes. Deleting a missing file is harmless. To delete a directory, list its file paths. Deleting the last file creates an empty tree. A path cannot appear twice or in both `files` and `deletes`.

For complete replacement, supply `replace: true` alongside the desired `files`. Only this mode removes omitted paths. `{"replace":true,"files":[]}` publishes an empty tree. Replacement cannot also specify `deletes`, and still supports `expected_head` and safe retries.

:::caution[Migration from earlier releases]
Earlier `POST /commits` requests replaced the entire tree implicitly. The default now preserves omitted files. Add `replace: true` to integrations that intentionally publish complete snapshots. Ordinary updates should keep the new default.
:::

## Retry without duplicating a publication

Send an `Idempotency-Key` header, such as a stable application operation ID. Repeating the same decoded request body and key against the same repository returns the original SHA and sequence, even if the branch subsequently advances. This works across restarts. Reusing a key with changed input returns `409` with kind `idempotency_conflict`.

Keys are 1–128 bytes without surrounding whitespace, NUL, CR, or LF. Successful results are retained indefinitely in Postgres in this implementation; there is no expiry window or cleanup API. Keys are scoped to the account, operation, and repository identity. Authentication is checked on every attempt: expired/revoked credentials and deleted repositories cannot use replay to regain access.

The result record and durable publication commit in the same database transaction. Failed attempts are not recorded and can be corrected/retried. Use the same key only for retries of the same intended operation. Supply a new key after changing the body to reconcile a conflict. Without a key, a lost response requires reading state before deciding whether to repeat the write.

## Fields and limits

| Field | Behavior |
| --- | --- |
| `files` | Paths to create/update with string `content`; may be omitted for a deletion-only commit. |
| `deletes` | Exact paths to remove; defaults to none. |
| `replace` | Defaults to false; true replaces the full tree. |
| `expected_head` | Optional full lowercase SHA; empty means the branch must not exist. |
| `base` | Optional full commit SHA for a new branch. |
| `branch` | Short branch name; defaults to the repository default. |
| `message` | Defaults to `Initial artifacts`. |
| `author` | Optional `name`, `email`, and RFC 3339 `date`; defaults to `Artifacts Agent`, `agent@artifacts.local`, and the current time. |

At most 100 combined writes and deletions, 1 MiB of decoded string content, and 2 MiB of JSON are accepted per request. At least one change is required unless `replace` is true. The repository itself can contain more files and bytes. UTF-8 byte length matters, not character count. Binary upload encodings and multipart uploads are not supported.

Paths must be normalized repository-relative paths. Traversal, absolute paths, `.git` components, duplicates, and file/directory collisions return `400` with kind `invalid_input`. Size limits return `413`. See [errors and limits](/artifacts/core/errors-and-limits/).

## Read the result

Use `/file?ref=SHA&path=...` or `/tree?ref=SHA` with the returned SHA. Pin related reads to the same commit. Objects are durable before refs become visible; cache reconstruction stays on the server.
