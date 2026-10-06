---
title: Write files
description: Update selected files atomically through REST, with version checks and safe retries.
---

REST and Git write the same repository history. REST accepts small text changes as JSON and larger text/binary changes as streaming multipart. Git supports local workspaces, larger file sets, and merges.

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
| `files` | JSON uses paths and string `content`; multipart uses paths and named raw file parts. May be omitted for a deletion-only commit. |
| `files[].mode` | Optional `100644` or `100755`; omission preserves an existing executable mode. |
| `deletes` | Exact paths to remove; defaults to none. |
| `replace` | Defaults to false; true replaces the full tree. |
| `expected_head` | Optional full lowercase SHA; empty means the branch must not exist. |
| `base` | Optional full commit SHA for a new branch. |
| `branch` | Short branch name; defaults to the repository default. |
| `message` | Defaults to `Initial artifacts`. |
| `author` | Optional `name`, `email`, and RFC 3339 `date`; defaults to `Artifacts Agent`, `agent@artifacts.local`, and the current time. |

At most 100 combined writes and deletions, 1 MiB of decoded string content, and 2 MiB of JSON are accepted per request. At least one change is required unless `replace` is true. The repository itself can contain more files and bytes. UTF-8 byte length matters, not character count. These are the JSON limits; multipart has separate limits below. JSON/base64 encoding is not supported.

Paths must be normalized repository-relative paths. Traversal, absolute paths, `.git` components, duplicates, and file/directory collisions return `400` with kind `invalid_input`. Size limits return `413`. See [errors and limits](/artifacts/core/errors-and-limits/).

## Publish binary and large files

Send `multipart/form-data` to the same `/commits` route. The first part must be named `manifest` with `Content-Type: application/json`. Its body contains commit metadata and file descriptors:

```json
{
  "expected_head": "0123456789abcdef0123456789abcdef01234567",
  "message": "Publish report",
  "files": [
    {"path": "report.md", "part": "text"},
    {"path": "report.pdf", "part": "pdf", "size": 104857600},
    {"path": "run.sh", "part": "script", "mode": "100755"}
  ],
  "deletes": ["draft.md"]
}
```

Use an actual observed SHA. Follow the manifest with raw `form-data` file parts named `text`, `pdf`, and `script`, in any order. Part identifiers must be unique ASCII letters, digits, underscores, or hyphens, 1–64 characters, excluding `manifest`. Repository paths come only from the manifest; a MIME filename is ignored. Files are stored as exact bytes, including NUL and non-UTF-8 data. MIME types are not stored in Git.

`size` is optional for generated streams. When supplied it must match the exact received byte count. Every stream is counted even when neither a declared size nor HTTP Content-Length is available. Modes accept only the strings `100644` and `100755`, with the same omission and symlink behavior as JSON writes. Missing, repeated, or extra parts, duplicate JSON members, unknown fields, nested multipart, and content-transfer encodings are rejected. Request compression is unsupported.

The default aggregate raw-content limit is **512 MiB**, configurable downward with `ARTIFACTS_COMMIT_MAX_BYTES`. A request still has at most 100 combined file writes and deletions. The manifest limit is 256 KiB; the total HTTP body limit is the configured content limit plus 2 MiB. Each MIME part's headers are limited to 8 KiB after the standard parser's bounded header parsing.

Uploads stream to private temporary files under `TMPDIR`; these files are immediately unlinked and released when their handles close, including on process exit. Two active multipart operations are admitted per process by default. Excess work receives 503 `upload_unavailable` with `Retry-After`. An idle stream expires after the configured stream timeout (30 seconds by default), and the complete multipart operation has a 15-minute default deadline. See [configuration](/artifacts/core/configuration/) for tuning.

All files and deletions publish atomically in one Git commit. Successful multipart requests return the same HTTP 201 envelope as JSON. Authentication is rechecked after receipt and before publication. Byte transfer completion alone is not proof of publication.

### Retry an upload

Send and retain an `Idempotency-Key` for the operation. After an interrupted transfer or lost response, resend the entire manifest and files with the same key. Multipart boundaries, part identifiers/order, filenames, and declared-size presence may change; verified bytes, paths, modes, and commit metadata must remain the same. Successful retries return the original SHA and sequence even if a later Git push advanced the branch. The transport must remain multipart; reusing the key with JSON conflicts.

There is no resumable upload session. A stream can only be retried if the caller can reproduce the original bytes. Keep input files unchanged until the outcome is known. A head conflict requires application reconciliation; use a new key for the resulting changed operation. A disconnected or timed-out finalization can have committed successfully, so recover through keyed replay rather than assuming rollback.

The [Node, Python, and curl examples](https://github.com/mikerudolph/artifacts/tree/main/examples/binary-commits) show streaming requests and client recovery. Node progress reports distinguish bytes consumed from the source, waiting for a response, and confirmed publication. These are example helpers, not an SDK.

### Provision disk and verify the workload

The content limit bounds ingress, not total disk or Git memory. Writes still use a full local Git cache, loose objects, a pack/index, and backend upload staging. Use disk-backed scratch/cache volumes with enough capacity for this amplification and repository history. A tmpfs consumes memory. Failed or competing attempts can leave durable unpublished packs; this feature does not add object garbage collection.

Validate the required size with the repository's `verify-binary` harness before choosing deployment limits. Client/proxy deadlines and request buffering must accommodate the workflow. A passing upload is not a throughput or peak-memory guarantee.

## Read the result

Use `/file?ref=SHA&path=...` or `/tree?ref=SHA` with the returned SHA. Pin related reads to the same commit. Objects are durable before refs become visible; cache reconstruction stays on the server.
