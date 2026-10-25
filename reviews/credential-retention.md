# Repository credential retention

Implemented locally on 2026-10-25 against `7329c32` for [issue #2](https://github.com/mikerudolph/artifacts/issues/2), on `codex/issue-2-credential-retention`. The [plan](credential-retention-plan.md) describes the design comparison. D17 records the policy; current operator guidance belongs in [authentication](../docs/core/authentication.md), [configuration](../docs/core/configuration.md), and [deployment](../docs/core/deployment.md).

## Behavior and ownership

Serving removes repository credential metadata seven days after expiry or first revocation, whichever occurs earlier. `ARTIFACTS_CREDENTIAL_RETENTION` accepts a nonnegative Go duration; zero disables pruning on that process. All serving instances should use the same setting. This changes metadata visibility, never credential validity. Removed IDs disappear from listings, direct revocation returns `404`, and authentication remains rejected. Control tokens and other retention concerns are unchanged.

Credential storage exposes one cleanup operation and owns eligibility, locking, and deletion. One SQL statement locks and deletes at most 1,000 eligible rows using `FOR UPDATE SKIP LOCKED`. The application runs one operation at startup and once per minute, with one five-second timeout and cancellation. A later tick retries remaining work or failures. Cleanup runs independently of compaction and joins before metadata closes. HTTP draining begins on termination without waiting for maintenance to join first.

Repository deletion now revokes credentials with one SQL update inside the existing repository transition transaction. This removes the offset-pagination race introduced by physical cleanup. Repository deletion still has a transaction covering all affected credentials; this change does not make that operation bounded.

## Design comparison

An owned PostgreSQL 16.4 fixture used 100,000 rows across 1,000 repository groups. It compared explicit expiry/revocation facts with one stored effective invalidation timestamp, first with a substantial invalid backlog and then with a mostly valid population. Both designs used indexes for bounded candidate selection. The query plans did not establish a material reason to duplicate expiry in another lifecycle column. Raw fixture output is preserved outside Git.

The chosen design adds internal `revoked_at`, preserves its first value on repeated revocation, and indexes `LEAST(expires_at, revoked_at)`. PostgreSQL's handling of the null revocation timestamp leaves expiry as the boundary for unrevoked rows. A constraint keeps revoked state and timestamp presence consistent. A separate `(repo_id, state)` index supports repository listing and counts. The final migrated-schema fixture showed indexed count/list queries and indexed selection followed by primary-key deletion for the complete cleanup statement. These small local fixtures do not establish production throughput or latency guarantees.

Migration 004 conservatively timestamps legacy revoked rows at migration time, while preserving expiry as a potentially earlier invalidation. Named-schema inspection requires the added column only at schema version 004 or later, allowing recognized older installations to upgrade. The backfill and index creation use the existing stop/migrate/restart deployment contract.

Cleanup uses metadata SQL only. It introduces no object-storage requests, transferred object bytes, or local repository materialization. That is an observation of the implementation boundary, not a measured improvement to storage performance.

## Verification

- Local formatting/imports, vet, lint, and Go file-size checks passed.
- The local race suite passed using an owned PostgreSQL server and uniquely created databases. Docker-dependent tests skipped. Regression tests cover cutoff boundaries, repeated revocation, batch limits, competing cleaners, locked rows, rollback, cancellation, migration upgrades, schema validation, and deletion/minting/cleanup races. Failed bulk revocation rolls back the repository transition and deletion job before a successful retry.
- All 24 local live assertions passed using two service processes with independent caches, shared owned filesystem objects, a named schema, and a restricted runtime role. With 2,408 old invalid rows, the two enabled instances each removed 1,000 at startup. A later scheduled attempt removed the remaining 408, observed at 60.2 seconds. Both nodes rejected removed credentials through REST; real Git rejected them while a valid credential continued to work. Neighboring application sentinels survived. Owned test resources were cleaned up.
- The verification skill's doctor and REST → Git → REST workflow passed, with classification `none`, eight passing assertions, and confirmed cleanup. Artifact digests and sizes were checked before preserving the outputs outside Git.
- At `b84caf2`, [CI](https://github.com/mikerudolph/artifacts/actions/runs/40168549752) passed the full `make verify` gate, all coverage thresholds, and 38 production container assertions. Total CI coverage was 90.1%. [Documentation checks](https://github.com/mikerudolph/artifacts/actions/runs/40168549744) also passed. CI stores the container evidence as a workflow artifact.

The test helper now accepts `ARTIFACTS_TEST_POSTGRES_URL` for an explicitly owned PostgreSQL server, creating and removing a unique database for each fixture. Database permission tests use unique roles and clean them up as well. The normal Docker fixture remains the default. This alternative allowed real PostgreSQL tests in this environment; it does not substitute for Docker/S3 coverage.

## Remaining verification limits

Local `make verify` stopped at the MinIO image build because Docker socket access was denied. Local total coverage was 88.9%; file/package gates failed for the skipped Docker and S3 paths. The later CI run above completed those checks. No threshold, exclusion, or test requirement was relaxed.

The local retention run used filesystem objects. CI verified the production image with Postgres and MinIO. No AWS target was used. These runs do not establish a new serving topology, production throughput, or a maximum deletion delay.

## CI follow-up

The first pushed revision, `c0aa304`, passed the documentation check and production container check. Its [verification job](https://github.com/mikerudolph/artifacts/actions/runs/40167705880/job/121092032131) passed the tests but failed package coverage. `internal/testkit` reached 82.6% against the required 85%. Total coverage reached 90.1%.

The isolation test called the new helper directly. It did not test selection through `ARTIFACTS_TEST_POSTGRES_URL` on the Docker runner. The follow-up changes that test to set the environment variable and call `Postgres`, the public fixture entry point. The same isolation assertions remain in place. Additional tests require a clear failure for a non-PostgreSQL URL and an unreachable database, before a test can use the wrong database. Coverage thresholds and exclusions remain unchanged. The PR contains the CI result for the latest pushed revision; the local evidence above remains a record of its original run.

## Evidence storage

PR #3 excludes its 19 raw evidence files. A verified local copy preserves the original output outside the repository. CI links above identify the tested revision and workflow artifacts. Artifact availability follows the CI retention policy. [Issue #4](https://github.com/mikerudolph/artifacts/issues/4) tracks the existing evidence directory and assessment of historical Git storage. Removing files from the PR diff does not purge blobs from earlier branch commits.
