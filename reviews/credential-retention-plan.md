# Issue #2: repository credential retention plan

Proposed 2026-10-25 against `7329c32`, on branch `codex/issue-2-credential-retention`. This is an implementation plan, not implemented behavior or verification evidence. Source: [issue #2](https://github.com/mikerudolph/artifacts/issues/2); its discussion contained no comments when inspected.

## Current behavior

- [Token storage](../internal/store/meta/postgres/tokens.go) inserts credentials, lists them by repository and optional state, and revokes by updating only `state`. Nothing deletes expired or revoked rows.
- [Migration 001](../migrations/001_init.up.sql) defines only primary-key and hash indexes for `repo_tokens`. Listing does a count and an offset query ordered by `created_at DESC`; neither has a repository index.
- [Authorization](../internal/auth/repository.go) checks stored repository binding, state, and expiry on every use. Expired credentials remain stored as `active`; that state currently means not explicitly revoked, rather than unexpired.
- [Repository deletion](../internal/jobs/delete.go) revokes credentials inside its deletion transaction by walking offset pages. Concurrent physical deletion can shift those pages or remove a selected credential before revocation.
- [Application maintenance](../internal/app/maintenance.go) starts background compaction through the serving lifecycle. Credential cleanup should have separate ownership and scheduling, so a slow compaction cannot delay it.
- [Schema inspection](../internal/store/meta/postgres/schema_setup.go) checks installation columns before migrations run. Adding a new required column to its unconditional column list would reject valid older installations before they can upgrade.

## Proposed contract

Use automatic background pruning in `serve` and `dev`. Configure `ARTIFACTS_CREDENTIAL_RETENTION` as a Go duration, default `168h` (seven days), with `0` disabling pruning. Reject malformed and negative values. The proposed default preserves a week of recent credential metadata for troubleshooting while allowing routine cleanup; it is a product policy choice, not a measured capacity requirement or an extension of credential access. Operators can choose a different visibility window or disable deletion when they need longer listing history. A maintenance outage or backlog can extend visibility beyond the configured window; retention is an eligibility threshold, not a deletion deadline.

A credential becomes eligible when its first invalidation is older than the retention cutoff:

- An unrevoked credential uses `expires_at`.
- A revoked credential uses the earlier of expiry and its first revocation time.
- An active, unexpired credential is never eligible.
- Exactly at the cutoff, retain the row until a later pass; use a strict older-than comparison.

Persist enough information to determine first invalidation atomically when revoking, and preserve cleanup eligibility on repeated revocation. The earlier-of rule prevents revoking an already expired credential from restarting its retention window. For legacy revoked rows, use the migration timestamp as their unknown revocation time; expiry may still make them eligible sooner. Do not invent an earlier historical revocation time from `created_at`.

Keep cleanup timestamps internal for this change; preserve current credential response shapes and state filters. Authentication stays rejected after physical removal, through the existing missing-token path. Credential listing and direct revocation can subsequently report an absent ID. These listings are operational metadata, not a permanent audit log.

The scope is repository credential rows only. Control tokens, idempotency results, repository erasure, object cleanup, and global retention scheduling remain separate work. Object-store transfers, requests, and local repository materialization should be unaffected; verify that cleanup calls only metadata storage.

## Ownership and design comparison

Credential storage owns eligibility evaluation, query shape, indexes, locking, and transactions. Expose one bounded cleanup operation accepting a context and cutoff and returning the deleted count. Keep the batch limit internal. The application supplies the retention cutoff and schedules calls; it must not select candidates, merge expiry and revocation results, acquire locks, or manage cleanup transactions. Keep repository-wide revocation inside the same storage boundary.

Before choosing migration columns and indexes, compare two complete designs against the same acceptance cases:

| Design | Benefit | Cost to evaluate |
| --- | --- | --- |
| Store the first `revoked_at` alongside the existing `expires_at`. | Explicit lifecycle facts; expiry remains the source for unrevoked credentials. | Cleanup must combine two timestamps; determine whether a simple predicate and supporting indexes suffice. |
| Store one effective invalidation timestamp, initialized to expiry and moved earlier on first revocation. | One cleanup timestamp and potentially one cleanup index. | Duplicates expiry information and requires every creation/revocation path to maintain the invariant atomically. |

Both must preserve the first-invalidation policy and conservative legacy handling above. Compare migration/backfill complexity, write-path changes, cleanup SQL, and representative query plans. Start with the simplest query for each design. Add separate candidate queries or de-duplication only if measurements demonstrate a need. Choose the design with the least total complexity, record the rationale, and then finalize migration 004. Neither a specific column representation nor an exact index set is selected by this plan.

## Implementation sequence

1. **Migration and upgrade safety.** After the design comparison, add migration 004 with the chosen internal timestamp representation, conservative backfill, and indexes justified by listing and cleanup queries. Include the down migration. Preserve valid pre-004 installation checks while validating the new shape after upgrade; if column requirements change, make that check aware of the recorded migration version. Preserve named-schema isolation and the current stop/migrate/restart deployment contract. Index creation and backfill may lock a large table during migration; do not claim an online upgrade.
2. **Revocation consistency.** Update revocation state and cleanup timing atomically in `repoTokenStore.Revoke` using the chosen timestamp representation; repeated revocation must not extend retention. Replace repository deletion's offset-based token walk with a store operation that revokes that repository's credentials in one SQL update within the existing repository transition transaction. This removes pagination shifts and read-then-revoke missing-row races. Preserve prior first-invalidation information. Test concurrent pruning, minting, and repository deletion; the existing ready-state repository lock must still prevent credentials being minted after deletion starts. Set-based revocation can lock many rows, as the current transaction already does; do not claim this step makes repository deletion bounded.
3. **Bounded store cleanup.** Implement the single cleanup operation with an internal row limit, keeping eligibility checks, row locks, and deletion in one transaction. Use `FOR UPDATE SKIP LOCKED` so competing cleaners can split work and leave busy rows for a later call. Match the first-invalidation rule without fetching token data into application memory. Select the query and indexes together from the design comparison. Multiple instances must work safely without a process leader or repository cache lock.
4. **Maintenance lifecycle and configuration.** Use one small cancellable loop, running one bounded cleanup operation asynchronously at startup and on each tick. Give each operation one timeout; do not add an inner drain loop, a separate pass deadline, or another retry mechanism. A later tick retries remaining work and database failures. Choose internal defaults for row limit, interval, and timeout, then validate their behavior against a representative backlog; expose only retention as operator configuration. Log failures and deleted counts without IDs, hashes, plaintext credentials, or DSNs. Start cleanup independently of compaction and join its goroutine before closing metadata. Additional serving instances multiply potential database work. Keep tests fast through direct cleanup calls with explicit cutoffs and a short private interval for lifecycle tests; introduce a test seam only where necessary, without a scheduler framework.
5. **Contract and decision documentation.** Update [authentication](../docs/core/authentication.md) beside TTL and listing guidance, [configuration](../docs/core/configuration.md) with retention/default/disable semantics, and [deployment](../docs/core/deployment.md) with migration/backlog considerations as needed. Narrow the existing statement that no automatic retention scheduler exists so it still describes deferred object/idempotency retention accurately. Append D17 when implementing this policy, recording scope, first-invalidation semantics, legacy timestamp handling, visibility changes, and limits; leave D1–D16 untouched.

Keep cleanup outside authentication, minting, listing, publication, and compaction request paths. Extract functions when they hide a meaningful responsibility; keep transaction and locking details together. Preserve the concurrency and migration tests below even as the scheduling design becomes smaller. Under D9, explanations belong in Markdown; do not add prose comments to implementation, SQL, tests, or documentation-site source. Keep the policy and ownership contract discoverable from the current authentication and configuration documentation.

## Verification and acceptance

Start with targeted tests, then run the required project gates once the implementation is complete.

| Area | Required evidence |
| --- | --- |
| Retention | Old expired and revoked rows disappear; recent invalid rows, exact-cutoff rows, and active unexpired rows survive. Cover an expired-then-revoked credential, revoke-before-expiry, and repeated revocation. |
| Migration | Fresh installation, repeated migration, version-003 upgrade with existing active/revoked rows, and older recognized installations. Test both default and named schemas, neighboring application preservation, and runtime permissions without schema ownership. |
| Batches and concurrency | More eligible rows than the internal limit; each call respects that limit; repeated calls drain the backlog; two independent cleanup connections; deliberately locked rows; cancellation; rollback and retry after database errors. Verify no duplicate accounting and eventual cleanup after locks release. |
| Repository lifecycle | More than one page of credentials, pruning during repository deletion, and concurrent minting/revocation. No skipped surviving credentials, deletion rollback due solely to pruning, or successful post-deletion mint. |
| Design and index use | Compare both timestamp designs with representative multi-repository data, including a backlog and a mostly unexpired population. Run `ANALYZE` and inspect `EXPLAIN (ANALYZE, BUFFERS)` for count/list with all states and a specific state, plus cleanup queries. Confirm relevant indexes are used without forcing planner settings. Tiny fixtures may legitimately use sequential scans. Record the selected design and index rationale. |
| Foreground work | Exercise minting, authentication, and REST/Git publication while cleanup runs. Confirm progress with held cleanup row locks and bounded cancellation. Report measurements and fixture size; avoid claiming zero contention or universal latency guarantees. |
| Lifecycle/config | Default, custom retention, zero disable, negative/malformed rejection, one bounded operation per tick, operation timeout, retry on a later tick, cancellation, and completion before pool close. Measure backlog progress with the chosen internal defaults. |
| Public behavior | Credential list metadata disappears after retention; removed credentials remain unauthorized on REST and Git; valid credentials still work with read/write scope and repository binding enforcement. |

Run `make verify` using isolated test Git configuration. Run `npm run check` from `docs-site/` after public documentation updates.

Use the [verify-artifacts skill](../.agents/skills/verify-artifacts/SKILL.md) for preflight and the real REST → Git → REST workflow on owned resources, plus retention-specific database assertions and two-instance cleanup coverage. Preserve redacted evidence and confirm cleanup. Local contention measurements do not establish production throughput or a maximum deletion delay. If a push is later authorized, inspect and wait for CI on the exact pushed revision under D11.

## Completion criteria

Issue #2 is ready to close when old invalid rows are pruned in bounded transactions, live credentials are preserved, representative list queries use supporting indexes, concurrent cleanup does not break repository deletion or delegated REST/Git access, operator documentation describes retention accurately, and all required verification passes. This planning change itself does not satisfy those criteria.

## Implementation outcome

The implementation and available verification are recorded in [credential retention](credential-retention.md). The selected design stores `revoked_at` and indexes `LEAST(expires_at, revoked_at)`, keeping cleanup timing derived from lifecycle facts. Docker access blocked the complete required verification gate; the implementation record distinguishes the completed PostgreSQL and REST/Git checks from that remaining limit.
