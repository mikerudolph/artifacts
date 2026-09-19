---
title: Write files
description: Publish a coherent file snapshot with REST, or use Git for incremental changes and binary content.
---

REST and Git write the same repository history. REST is convenient for seeding a workspace or publishing a small, fully generated text bundle. Git fits updates to an existing workspace, larger file sets, and binary outputs.

## The REST snapshot contract

`POST /commits` creates a commit whose file tree consists of the files in your request. **It is a complete snapshot, not a file patch.** Existing paths you omit disappear from the new tree; older commits still retain them.

| Before | Files in your next request | After |
| --- | --- | --- |
| `brief.md`, `report.md` | Updated `report.md` only. | `report.md` only; `brief.md` is removed. |
| `brief.md`, `report.md` | Original `brief.md` and updated `report.md`. | Both files, with the updated report. |
| No commits. | `brief.md`, `report.md`. | A first commit containing both files. |

If your application cannot supply the whole intended tree within the limits below, use [Git](/artifacts/core/git/). There is no REST append, single-file update, multipart upload, or base64 decoding field.

## Publish a snapshot

Create a repository and set `REPO_API` as in [manage repositories](/artifacts/core/repositories/). Then publish:

```bash
curl --fail-with-body -sS -X POST "$REPO_API/commits" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "branch": "main",
    "message": "Publish the launch report",
    "author": {"name": "Research worker", "email": "worker@example.com"},
    "files": [
      {"path": "inputs/brief.md", "content": "# Brief\nResearch the launch plan.\n"},
      {"path": "outputs/report.md", "content": "# Report\nThe launch plan is ready.\n"}
    ]
  }' | jq
```

The result is HTTP `201` with this shape (the SHA below is illustrative):

```json
{
  "result": {"sha": "0123456789abcdef0123456789abcdef01234567", "sequence": 1},
  "success": true,
  "errors": [],
  "messages": []
}
```

Store `sha` when you need to refer to this exact version. The publication sequence increases for durable writes and is useful for diagnostics; it is not the number of files or commits in a Git push.

## Fields and defaults

| Field | Behavior |
| --- | --- |
| `files` | Required array of 1–100 objects with `path` and string `content`. |
| `branch` | Short branch name. Omitted or empty selects the repository default. |
| `message` | Defaults to `Initial artifacts` if omitted or empty. |
| `author.name` | Defaults to `Artifacts Agent`. |
| `author.email` | Defaults to `agent@artifacts.local`. |
| `author.date` | Optional RFC 3339 timestamp; defaults to the current time. Used for author and committer. |

On an existing branch, the commit's parent is that branch's current head. A REST write to a new branch creates a new root commit; it does not implicitly branch from `main`. Use Git when you want to branch from an existing commit, or [fork a repository](/artifacts/core/forks-and-imports/) for independent access.

## Validate before sending

The decoded file contents may total at most **1 MiB**. The complete JSON request may be at most **2 MiB**, including paths, metadata, and JSON escaping. UTF-8 byte length matters, not the number of characters. Every path must be unique and a normalized repository-relative file path, such as `outputs/report.md`.

Reject traversal, absolute paths, `.git` paths, duplicates, and file/directory collisions in your client. Avoid platform-specific path separators. The endpoint rejects unknown JSON fields and additional JSON values after the first body.

The API currently maps some semantic commit validation failures to generic `500` errors. Do not treat those as retryable transport problems. See the full [limits and error behavior](/artifacts/core/errors-and-limits/).

## Coordinate writers and retries

Artifacts validates ref and publication state when publishing, but the REST endpoint has no client-supplied expected SHA, merge operation, or idempotency key. A successful write does not prove your snapshot incorporated another worker's latest work.

Use one writer per repository/branch, serialize snapshot updates in your application, or give independent workers snapshot forks. Use Git fetch and merge when several writers intentionally share a history. A failed Git push needs reconciliation rather than an automatic force push.

After a timeout, read the branch and expected content before retrying. Publication may have succeeded even when your client missed the response, and replaying a request can create another commit.

## Read back what you published

Resolve the returned `sha` with `/file?ref=SHA&path=...` or `/tree?ref=SHA`. Pin all reads for one result to the same commit. A successful publication makes its objects durable before its refs become visible; a cache miss may require server-side reconstruction before the read completes.

Continue to [read files and history](/artifacts/core/reading-files/) for URL encoding, tree traversal, and versioned reads.
