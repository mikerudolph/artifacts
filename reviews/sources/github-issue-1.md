# GitHub issue 1 source snapshot

Source: [Integration requests: erasure, binary REST writes, AWS deployment, read-path improvements](https://github.com/mikerudolph/artifacts/issues/1)

| Snapshot field | Value |
| --- | --- |
| Author | mikerudolph |
| State at retrieval | open |
| Created | 2026-09-29T13:52:05Z |
| Last updated | 2026-09-29T13:52:05Z |
| Retrieved | 2026-10-03 |
| Comments | 0, confirmed by fetching the comment list |
| Labels | enhancement |

The issue body below is preserved verbatim. This is a historical source snapshot, not a statement of implemented behavior. See [the feasibility assessment](../integration-requests.md) for proposed work.

---

We're evaluating Artifacts as the versioned-file store behind a multi-tenant application backend. This issue collects the changes that would let us adopt it. It is ordered by how much each item blocks production use, and each item has an acceptance check in the style of `GOALS.md`. Observations are against `f59a410`.

### How we would use it

- **Access:** the backend (TypeScript/Node) is the only holder of the control token and talks to Artifacts over REST only. Git access for delegated workers may come later.
- **Layout:** one account per environment, one namespace per tenant, and one repository per logical bundle.
  - A bundle is a small, user-authored, multi-file folder: markdown and text files, occasionally small binary assets.
  - We also plan to store generated outputs, some of them binary.
- **Consistency:** writes use `expected_head` and `Idempotency-Key`. The backend records the commit SHA on its own records and caches content by SHA, so hot-path reads never hit Artifacts.
- **Deployment:** AWS. Kubernetes with no persistent node-local disk (the cache is ephemeral pod storage), managed Postgres, and S3. Our environment mandates IAM token auth for Postgres and SSE-KMS on buckets.

The core model already fits well. Incremental commits, `expected_head` conflicts, durable idempotency and SHA-pinned reads are exactly the contract we need. The items below are the gaps.

---

## P0: blocks storing customer content

### 1. Physical erasure: purge a repository or namespace

**Problem.** `DELETE` tombstones a repository and revokes its credentials, but everything else stays indefinitely: packs, indexes, checkpoints, WAL, ref and publication rows, and `idempotency_results` (including result JSON). There is no namespace delete. We can't honor tenant offboarding, deletion requests, or cleanup after a secret was committed by mistake.

**Proposal.**
- An explicit, lineage-aware purge, such as `POST …/repos/{repo}/purge`. It removes the repository's objects under its storage prefix and its metadata: WAL, ref updates, events, checkpoints, credentials and idempotency records.
- If snapshot forks depend on the repository's packs, refuse with `409` listing the dependents. Materializing the children first is an alternative, but refusing is enough for us.
- A namespace purge that applies the same operation to every repository in the namespace, then removes the namespace.
- A durable, resumable job for this, since object deletion can be interrupted.
- Retention for idempotency records (for example `ARTIFACTS_IDEMPOTENCY_RETENTION`), plus GC for uploaded-but-never-published packs.

Repository-level purge is enough for single-file erasure too: we would publish a sanitized head into a fresh repository and purge the old one.

**Acceptance.**
- After purge, no object remains under the repository's storage prefix and no metadata row references its ID.
- REST, Git, events and idempotency replays all return `404`.
- A purge interrupted mid-way completes when retried.
- Purging a repository that has live fork descendants is refused, or the descendants still reconstruct.

### 2. Binary content in REST commits

**Problem.** `files[].content` accepts UTF-8 strings only. Publishing a small image or PDF requires running a Git client inside an otherwise REST-only backend.

**Proposal.**
- Add `files[].encoding: "utf-8" | "base64"`, defaulting to `utf-8`, with limits applied to decoded bytes.
- Add an optional `files[].mode` (`100644` / `100755`).
- If the 2 MiB JSON cap stays, make it configurable so base64 payloads fit the decoded-content limit.

This matches item 11 of `reviews/developer-interface-review.md`.

**Acceptance.** A markdown file and a PNG publish together in one commit. `/file` returns byte-identical content for both at that SHA, and an executable mode round-trips.

---

## P1: deployment on AWS

### 3. Postgres IAM token authentication

**Problem.** The metadata store opens `pgxpool.New(ctx, dsn)` with a static DSN. Environments that require RDS IAM auth, where tokens expire every 15 minutes, can't connect with a passwordless role.

**Proposal.** Add an opt-in mode such as `ARTIFACTS_DATABASE_AUTH=rds-iam`:
- It uses a pgx `BeforeConnect` hook to mint a token with `aws-sdk-go-v2/feature/rds/auth` through the default credential chain (IRSA or task role).
- It requires TLS.
- It applies to `serve`, `migrate` and `bootstrap` alike.

**Acceptance.** All three commands work with a role that has `rds_iam` and no password. A long-running server keeps opening new connections after the first token has expired.

### 4. S3 server-side encryption settings

**Problem.** `PutObject` sends no SSE headers. Buckets whose policy denies uploads without `x-amz-server-side-encryption` reject every write.

**Proposal.**
- Add `S3_SSE` (`aws:kms` | `AES256`) and an optional `S3_SSE_KMS_KEY_ID`, and apply them to every write.
- Optionally add a startup check that performs one encrypted write, since `/readyz` doesn't verify write permission today.

**Acceptance.** It works against a bucket with a deny-unencrypted-put policy. Behavior is unchanged when the settings are unset, as in the MinIO tests.

### 5. Rolling upgrades

**Problem.** The docs require stopping all old instances before migrating and starting the new release. On Kubernetes that means `strategy: Recreate` and a write outage on every deploy. In addition, `ARTIFACTS_SKIP_MIGRATIONS=true` rejects newer schemas, which blocks the usual migrate-then-roll sequence.

**Proposal.**
- An expand/contract migration policy, so release N's schema stays compatible with release N-1 binaries.
- Serving accepts any schema within a declared compatibility range.
- Release notes flag any upgrade that needs a full stop.

**Acceptance.** A rolling update from N-1 to N under continuous REST writes, with no failed publications other than retriable `409`/`503` responses.

### 6. Observability

**Problem.** There is no metrics endpoint and very little logging, so an operator can't alert on publication failures, cache rebuild storms or compaction backlog.

**Proposal.**
- Prometheus `/metrics` or OpenTelemetry, covering:
  - request count and latency by route and status
  - publication latency and outcome
  - cache hits, rebuilds and rebuild duration
  - object-store latency and errors
  - lock wait time
  - repositories above the compaction threshold
- Structured request logs carrying a request ID, with `X-Request-Id` / `traceparent` passthrough.

**Acceptance.** Each of the conditions above can be alerted on from the exported signals alone.

---

## P1: read path

### 7. Commit identity and conditional GET on content reads

**Problem.** `/file` returns bytes with no indication of which commit or blob was read. To learn the SHA it read, a caller must resolve the ref with `/tree` first. Without an ETag, standard HTTP caching isn't possible.

**Proposal.**
- Return `X-Artifacts-Commit: <commit sha>`, `X-Artifacts-Blob: <blob sha>` and `ETag: "<blob sha>"`.
- Honor `If-None-Match` with a `304`.
- Send `Cache-Control: immutable` when `ref` is a full commit SHA.

**Acceptance.** Reading `ref=main` reports the commit it resolved. A repeated read with `If-None-Match` returns `304` with no body.

### 8. Reading a folder in one request

**Problem.** `/tree` lists a single directory and `/file` returns a single file. Reading a 20-file folder at one SHA takes one request per directory plus one per file, and each request pays the full per-request read cost.

**Proposal.**
- Add `/tree?recursive=true`, returning a flat list with full paths and sizes.
- Add either `GET /archive?ref=&path=&format=tar` or a bounded batch read that returns several paths from one resolved commit.

**Acceptance.** A 20-file folder is read at one commit in at most two requests, and every file is guaranteed to come from the same commit.

### 9. Concurrent reads of one repository

**Problem.** Content reads take the exclusive per-repository mutex and `flock(LOCK_EX)` and hold it while the response body streams (`Manager.ReadContent` → `lockedPath`). Concurrent reads of the same repository serialize on each instance, and a slow client can hold the lock until the idle timeout.

**Proposal.**
- Take a shared lock for reads on the range-read path, and keep the exclusive lock for rebuild, receive, REST commit and compaction.
- Alternatively, release the lock once the object reader is resolved and small blobs are buffered.

**Acceptance.** N concurrent reads of one repository finish in roughly the time of one read, and the existing rebuild and concurrency tests still pass.

---

## P2: quality of life

10. **Path-scoped history.** Add `/log?path=`, returning commits that touch a path, like `git log -- <path>`.
11. **Fork targets.** Allow forking into another namespace in the same account (control token only), and forking at a given commit SHA (review item 10). This covers copying a shared template into a tenant namespace while keeping its history.
12. **Machine-readable contract.** Add an OpenAPI description checked against the mounted routes in CI, so consumers can generate typed clients. We don't need an official SDK. Two consistency fixes would also help: return `[]` instead of `null` for empty lists, and use one pagination style for new list endpoints.
13. **Control-token lifecycle.** Add list, revoke and optional expiry for control tokens in the CLI, so tokens can be rotated without editing the database.

## Tracking

- [ ] 1. Physical erasure (repository and namespace purge, idempotency retention)
- [ ] 2. Binary content in REST commits
- [ ] 3. Postgres IAM token authentication
- [ ] 4. S3 server-side encryption settings
- [ ] 5. Rolling upgrades
- [ ] 6. Observability
- [ ] 7. Commit identity and conditional GET on content reads
- [ ] 8. Reading a folder in one request
- [ ] 9. Concurrent reads of one repository
- [ ] 10. Path-scoped history
- [ ] 11. Fork targets
- [ ] 12. Machine-readable contract
- [ ] 13. Control-token lifecycle

## Explicitly not needed

- An official SDK
- Webhooks: polling `repos?sort=last_push_at` is enough
- Authenticated upstream imports: we fetch upstream content ourselves and publish it through REST
- Path-level permissions
- Changes to the web UI
