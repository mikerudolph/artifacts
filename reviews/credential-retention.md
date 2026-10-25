# Repository credential retention

Implemented locally on 2026-10-25 against `7329c32` for [issue #2](https://github.com/mikerudolph/artifacts/issues/2), on `codex/issue-2-credential-retention`. The [plan](credential-retention-plan.md) describes the design comparison. D17 records the policy; current operator guidance belongs in [authentication](../docs/core/authentication.md), [configuration](../docs/core/configuration.md), and [deployment](../docs/core/deployment.md).

## Behavior and ownership

Serving removes repository credential metadata seven days after expiry or first revocation, whichever occurs earlier. `ARTIFACTS_CREDENTIAL_RETENTION` accepts a nonnegative Go duration; zero disables pruning on that process. All serving instances should use the same setting. This changes metadata visibility, never credential validity. Removed IDs disappear from listings, direct revocation returns `404`, and authentication remains rejected. Control tokens and other retention concerns are unchanged.

Credential storage exposes one cleanup operation and owns eligibility, locking, and deletion. One SQL statement locks and deletes at most 1,000 eligible rows using `FOR UPDATE SKIP LOCKED`. The application runs one operation at startup and once per minute, with one five-second timeout and cancellation. A later tick retries remaining work or failures. Cleanup runs independently of compaction and joins before metadata closes. HTTP draining begins on termination without waiting for maintenance to join first.

Repository deletion now revokes credentials with one SQL update inside the existing repository transition transaction. This removes the offset-pagination race introduced by physical cleanup. Repository deletion still has a transaction covering all affected credentials; this change does not make that operation bounded.

## Design comparison

The [comparison fixture](evidence/credential-retention-2026-10-25/query-comparison.sql) used 100,000 rows across 1,000 repository groups on task-owned PostgreSQL 16.4. It compared explicit expiry/revocation facts with one stored effective invalidation timestamp, first with a substantial invalid backlog and then with a mostly valid population. Both designs used indexes for bounded candidate selection. The [plans](evidence/credential-retention-2026-10-25/query-comparison.txt) did not establish a material reason to duplicate expiry in another lifecycle column.

The chosen design adds internal `revoked_at`, preserves its first value on repeated revocation, and indexes `LEAST(expires_at, revoked_at)`. PostgreSQL's handling of the null revocation timestamp leaves expiry as the boundary for unrevoked rows. A constraint keeps revoked state and timestamp presence consistent. A separate `(repo_id, state)` index supports repository listing and counts. The [final migrated-schema fixture](evidence/credential-retention-2026-10-25/query-final.sql) and [execution plans](evidence/credential-retention-2026-10-25/query-final.txt) show indexed count/list queries and indexed selection followed by primary-key deletion for the complete cleanup statement. These small local fixtures do not establish production throughput or latency guarantees.

Migration 004 conservatively timestamps legacy revoked rows at migration time, while preserving expiry as a potentially earlier invalidation. Named-schema inspection requires the added column only at schema version 004 or later, allowing recognized older installations to upgrade. The backfill and index creation use the existing stop/migrate/restart deployment contract.

Cleanup uses metadata SQL only. It introduces no object-storage requests, transferred object bytes, or local repository materialization. That is an observation of the implementation boundary, not a measured improvement to storage performance.

## Verification

- [Static checks](evidence/credential-retention-2026-10-25/static-final.log): formatting/imports, vet, and lint passed with zero lint issues. [Go file-size checks](evidence/credential-retention-2026-10-25/complexity.log) passed.
- [Race and coverage run](evidence/credential-retention-2026-10-25/coverage-final.log): `go test ./... -race -count=1 -coverprofile=... -covermode=atomic -coverpkg=./...` passed using an owned PostgreSQL server and uniquely created databases. Docker-dependent tests skipped. New tests cover cutoff boundaries, repeated revocation, initially revoked rows, batch limits, competing cleaners, locked rows, rollback, canceled and blocked SQL, migration upgrade/downgrade/re-upgrade, schema validation, cleanup lifecycle, and deletion/minting/cleanup races. Failed bulk revocation rolls back the repository transition and deletion job before a successful retry.
- [Live checks](evidence/credential-retention-2026-10-25/live-pass2/checks.json): all 24 assertions passed using two real service processes with independent caches, a shared owned filesystem object store, a named schema, and a runtime role without schema creation permission. Zero disabled startup pruning. With 2,408 old invalid rows, the two enabled instances each removed 1,000 at startup, leaving 408; a later scheduled attempt removed the rest, observed at 60.2 seconds. Both nodes rejected removed credentials through REST; real Git rejected the same credentials while a valid credential continued to work. Neighboring application sentinels survived. The owned repository, live database, roles, and service processes were cleaned up.
- The verification skill's [doctor](evidence/credential-retention-2026-10-25/live-pass2/doctor.txt) passed, followed by [REST → Git → REST](evidence/credential-retention-2026-10-25/live-pass2/core/report.md), with classification `none`, eight passing assertions, and confirmed repository cleanup. Artifact digests and sizes were checked.
- [Documentation-site checks](evidence/credential-retention-2026-10-25/docs-check-final.log) passed.

The test helper now accepts `ARTIFACTS_TEST_POSTGRES_URL` for an explicitly owned PostgreSQL server, creating and removing a unique database for each fixture. Database permission tests use unique roles and clean them up as well. The normal Docker fixture remains the default. This alternative allowed real PostgreSQL tests in this environment; it does not substitute for Docker/S3 coverage.

## Remaining verification limits

The required [make verify run](evidence/credential-retention-2026-10-25/make-verify-final.log) passed formatting, vet, and lint, then failed to build the pinned MinIO fixture because Docker socket access was denied. [Coverage evaluation](evidence/credential-retention-2026-10-25/coverage-gate-final.log) reports 88.9% total coverage, but the unchanged file/package gates fail for S3 and the Docker fixture paths skipped here. No threshold, exclusion, or test requirement was relaxed. The complete gate and built-image deployment check remain unverified until Docker is available.

No AWS target, S3 deployment, or container image was used for the live retention run. The two processes shared a task-owned filesystem object store, so that run is evidence for cleanup/authentication behavior and independent caches, not qualification of a new serving topology. No production performance or maximum deletion-delay claim follows. No push was made, so remote CI has not run for these uncommitted changes.

An [earlier live attempt](evidence/credential-retention-2026-10-25/live/checks.json) stopped after the verification driver sent `name` instead of `namespace` when creating a namespace. It received `400` and confirmed owned database/role cleanup; this was a driver error. The corrected attempt above passed. Earlier package runs also exposed a cleanup context error and legacy test fixtures missing migration 004; both were corrected before the final race run.
