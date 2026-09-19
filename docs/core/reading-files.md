---
title: Read files & history
description: Resolve a version, browse its tree, and retrieve exact file bytes without cloning a repository.
---

Your application can read Git-published content directly through REST. The server reconstructs a missing or stale repository cache as needed; clients never fetch or unpack storage WAL objects.

The examples use `REPO_API` for an existing repository's REST URL and `CONTROL_TOKEN` for its account. Set them as in [manage repositories](/artifacts/core/repositories/).

## Resolve a ref and list a directory

```bash
TREE=$(curl --fail-with-body -sS --get "$REPO_API/tree" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  --data-urlencode 'ref=main' --data-urlencode 'path=outputs')
printf '%s' "$TREE" | jq '.result'
export COMMIT_SHA=$(printf '%s' "$TREE" | jq -er '.result.commit')
```

The JSON result contains:

| Field | Meaning |
| --- | --- |
| `ref` | Requested ref, or `HEAD` when omitted. |
| `commit` | Resolved commit SHA. Reuse it for consistent follow-up reads. |
| `tree` | Git tree object SHA for the selected directory. |
| `path` | Selected directory path; empty string for the root. |
| `entries` | Direct children, each with `name`, `mode`, `type`, and `hash`. |

Omit `path` to list the root. A directory entry has type `tree`; a file entry usually has type `blob`. This is a single-directory listing, not a recursive walk. Request each child directory separately, using the same commit SHA.

## Read exact file bytes

```bash
curl --fail-with-body -sS --get "$REPO_API/file" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  --data-urlencode "ref=$COMMIT_SHA" \
  --data-urlencode 'path=outputs/report.md'
```

A successful file response is `application/octet-stream`. It contains the original bytes, even for Markdown or JSON. Do not parse it as the API envelope; parse it as your file format only after checking the HTTP status.

Use `--data-urlencode` or `URLSearchParams` for paths containing spaces, `#`, `&`, or other reserved characters, and for branch names containing `/`. To save a binary file, use curl's `--output` option with your intended destination.

```js title="A server-side reader using fetch"
const query = new URLSearchParams({ ref: commitSha, path: 'outputs/report.md' });
const response = await fetch(`${repoApi}/file?${query}`, {
  headers: { Authorization: `Bearer ${controlToken}` },
});
if (!response.ok) throw new Error(`File read failed: HTTP ${response.status}`);
const report = await response.text(); // Use arrayBuffer() or a stream for binary data.
```

Here `commitSha`, `repoApi`, and `controlToken` come from your backend's repository mapping and configuration. Treat returned content as untrusted input when displaying HTML or consuming it with an agent.

## Pin a version for related reads

Ref resolution accepts `HEAD`, a branch such as `main`, a full ref such as `refs/heads/main`, a tag name, or a full 40-character object SHA. File, tree-at-ref, and history operations require that the resolved object is a commit. Use a commit SHA when you need an exact version; do not assume arbitrary Git revision expressions such as `main~2` are supported.

The reader does not peel annotated tag objects. Use a lightweight tag that points directly to a commit, or supply the underlying commit SHA.

For a result that includes a manifest and several output files:

1. Resolve the branch once with `/tree?ref=main` and retain `result.commit`.
2. Read the manifest at that commit SHA.
3. Read every file referenced by the manifest at the same SHA.
4. Record that SHA in your application's result or search index.

This keeps the result internally consistent when another writer advances the branch. Repeated independent reads of `main` do not provide a shared snapshot.

## Inspect history and objects

```bash
curl --fail-with-body -sS --get "$REPO_API/log" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  --data-urlencode 'ref=main' --data-urlencode 'limit=20' \
  --data-urlencode 'offset=0' | jq '.result'

curl --fail-with-body -sS "$REPO_API/commit/$COMMIT_SHA" \
  -H "Authorization: Bearer $CONTROL_TOKEN" | jq '.result'
```

The log lists commit hashes, messages, authors, and committers. It defaults to 20 entries and offset 0. The commit endpoint also includes its tree SHA and parent hashes. Use `/tree/{tree_sha}` for a tree object or `/blob/{blob_sha}` for a blob; these hashes are different object types and are not interchangeable.

`/raw/{ref}/{path}` returns bytes with a detected content type. Prefer the query-based `/file` endpoint when refs contain slashes or when you want a download without inline content-type interpretation.

## Handle empty and missing content

An empty repository has no commit to resolve. Missing refs, directories, and files return `404`, as does normal lookup of a deleted repository. A missing file is not the same as a successful response containing zero bytes.

Cold reads can take longer because the server materializes the repository. Set client deadlines appropriate to your workload. For raw streams, also detect an interrupted response rather than treating partial bytes as a complete file. If your file manifest records a length and digest, verify both before accepting the output.

See [API reference](/artifacts/core/api-reference/#content-and-history) for the complete read surface.
