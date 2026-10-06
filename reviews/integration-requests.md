# Integration request feasibility and delivery plan

Assessed 2026-10-03 against `f59a41024dd49e4e9e044695206d7e2e49bc5657`, which is also the revision cited by [GitHub issue 1](https://github.com/mikerudolph/artifacts/issues/1). The [verbatim issue snapshot](sources/github-issue-1.md) preserves all 13 requests, acceptance criteria, and exclusions. GitHub reported the issue open with no comments when retrieved.

Updated 2026-10-05 for the user's requirement that binary REST writes support files and commits in the tens or hundreds of MiB. Request 2 now recommends streaming multipart and a staged implementation; see the [focused binary-write proposal](binary-rest-writes.md). The other request assessments retain their original scope.

All 13 requests have viable implementation paths within the product's versioned-file model, with the contract clarifications below. Physical erasure and rolling upgrades need new lifecycle contracts and substantial failure testing. Concurrent reads and pinned forks need careful snapshot ownership. The other requests can mostly extend existing boundaries.

The recommended path is to settle erasure and upgrade contracts first, ship contained integration improvements, build the purge lifecycle, and then extend reads and forks. Establish the upgrade foundation before schema changes if rolling deployment is required throughout delivery. Keep OpenAPI and instrumentation alongside feature work. Repository purge, binary writes, IAM authentication, and S3 encryption are the adoption blockers for the described customer, regardless of their different issue priority labels.

This is a proposal based on source and test inspection, not an implementation or a performance result. No decision has been appended to [AGENTS.md](../AGENTS.md); D1–D13 remain authoritative. No issue comments, child issues, commits, or pushes were created.

## Feasibility at a glance

Size describes implementation and verification scope, not elapsed time: **S** is a contained change; **M** spans several components; **L** needs multiple coordinated changes and substantial failure coverage; **XL** introduces an operational lifecycle or release qualification program. Splitting a row into several deliveries does not complete the original request until all its acceptance criteria are met.

| Request | Size | Assessment | Main decision or dependency |
| --- | --- | --- | --- |
| 1 Physical erasure | XL | Feasible after defining the erasure boundary and resumable operation model | Fork dependencies, active work, storage versions, local caches, replay retention |
| 2 Binary REST writes | L | Feasible with bounded streaming and commit-path refactoring | Multipart manifest, disk admission, short publication transactions, stable retry digests |
| 3 Postgres IAM auth | M | Supported by the connection libraries already used | Shared connection configuration for pooled and migration connections, verified TLS |
| 4 S3 encryption | S | Centralized upload path makes this contained | Allowed settings, KMS permissions, handling existing objects |
| 5 Rolling upgrades | XL | Feasible for explicitly qualified release pairs | Bootstrap release, schema compatibility, background worker and storage compatibility |
| 6 Observability | M–L | Clear instrumentation points exist | Metric backend, bounded labels, scrape access, publication versus request outcomes |
| 7 Read identity and conditional GET | S–M | Refactor file resolution and response headers | Authorization before 304, private cache lifetime, blob versus commit identity |
| 8 Folder reads | M–L | Feasible using one captured snapshot | Bounded batch versus archive, tree sizes, pagination, special Git entries |
| 9 Concurrent reads | L | Feasible on the immutable range path | Index repair and large-object fallback still mutate local state |
| 10 Path history | M | Feasible; exact Git history semantics increase scope | Literal paths, merge rules, rename behavior, bounded traversal |
| 11 Fork targets | M for namespace; L for pinned commits | Namespace routing is contained; pinning needs a precise contract | Captured publication, child refs, retained objects, purge coordination |
| 12 OpenAPI and consistency | M | Can start immediately and grow with each feature | Contract ownership, route parity, wire examples, new pagination convention |
| 13 Control-token lifecycle | M | Straightforward schema and CLI extension | Permanent revocation, expiry, bootstrap behavior, rotation without lockout |

## Request assessments

### 1 Physical erasure

**Current behavior.** [DeleteRepo](../internal/jobs/delete.go) transitions a repository to deleted and revokes repository credentials transactionally. It does not remove object data. The [storage schema](../migrations/002_v2_storage.up.sql) retains WAL, checkpoints, and fork lineage; several foreign keys deliberately prevent simply deleting the repository row. [Jobs](../internal/store/meta/postgres/jobs.go) have no claiming or recovery worker even though migration 002 added unused lease, payload, and attempt columns. [Idempotency records](../migrations/003_idempotency.up.sql) have neither typed repository ownership nor expiry. The existing S3 [DeletePrefix](../internal/store/object/s3/list.go) collects every current key into memory and deletes individually.

**Recommended implementation.** Preserve ordinary DELETE as tombstoning. Add an explicit control-token-only purge operation, returning an operation ID with HTTP 202, and an account-scoped operation lookup that survives the target becoming inaccessible. Build it in separately reviewable parts:

1. Persist purge intent and atomically fence the repository against new publication, imports, forks, credential issuance, and settings changes. Serialize the dependency check with fork creation in Postgres. Refuse with a bounded, paginated 409 dependency result while any unpurged child lineage remains. Conservatively include tombstoned children and transitive dependencies; checking only ready direct children is insufficient.
2. Claim operations with database leases, bounded batches, retry state, and restart recovery. Existing job columns are useful groundwork, but the job API and worker do not yet implement these responsibilities. Distinguish lease expiry from proof that an old worker stopped. Drain or cancel tracked in-flight work, including compaction and uploads, before declaring storage empty. Publication status checks alone cannot prevent an upload that started earlier from finishing after a delete sweep.
3. Delete objects using a paginated storage interface and resumable progress. Keep repository identity and purge intent until storage work is complete. Remove dependent metadata in the correct order, including create results that reference the repository, commit results, jobs, refs, credentials, checkpoints, lineage, and WAL/ref updates. Events derive from WAL; there is no separate event table to erase.
4. Purge a namespace by first fencing creation/import/fork into it, checking external descendants, and capturing its repositories, including deleted and failed ones. Process children before parents within that set, then remove the namespace. Report partial progress and retry it; do not promise an atomic rollback of completed object deletions.

**Erasure boundaries to choose.** S3 versioning requires deletion of object versions and delete markers, not just current keys. The existing backend's DeleteObject call cannot prove physical erasure on a versioned bucket. Version enumeration and deletion must be supported, or purge must explicitly reject that configuration. Retention locks and insufficient permissions must keep the operation incomplete. [AWS deletion semantics](https://docs.aws.amazon.com/AmazonS3/latest/userguide/DeletingObjectVersions.html) explain this distinction.

The service also leaves bytes in independent Git caches, index directories, upload scratch files, and possibly compaction scratch space. A successful API denial does not erase them. Define a completion barrier for all active instances and a startup cleanup requirement for returning instances, or explicitly qualify completion as durable-store deletion with separately bounded local cleanup. An offline disk, database backups, database WAL/PITR, bucket replicas, and consumer caches need an operator retention contract; deleting application rows cannot establish their erasure. Requiring immediate verified erasure of every offline copy would need a larger design, potentially including key management beyond shared SSE-KMS.

**Replay conflict in the acceptance criteria.** [CreateRepo](../internal/service/repo.go) can replay a cached result before rechecking the resource, and can implicitly create a missing namespace. Deleting its idempotency result outright permits a later retry to create a new repository. Permanent 404 for every historical create retry cannot coexist with forgetting all replay history and allowing namespace/name reuse. Recommend erasing result payloads and retaining a bounded suppression record keyed by an opaque request fingerprint, with no plaintext repository ID or content after finalization. Choose its lifetime, treatment of names, and post-expiry behavior explicitly. Define a minimal completed operation receipt separately from repository metadata so a lost purge response remains recoverable.

The issue's blanket 404 also needs qualification: current REST repository credentials receive 401 when authorization cannot resolve their repository, while authenticated control-token lookups and Git repository resolution return 404. Preserve authentication behavior unless deliberately changing that contract. Acceptance should prove there is no content or replay access, with response codes specified per authentication path.

**Retention and orphan cleanup are separate deliveries.** Add explicit repository/namespace ownership to live idempotency records, with a migration/backfill for existing create results and commit scopes. Keep the current indefinite default initially; make finite retention opt-in with a documented retry window and bounded deletion batches. Expired keys no longer guarantee replay. Purge removes result content regardless of normal retention.

For uploaded-but-unpublished packs, introduce independently committed upload intent/leases before transfers, plus reference-aware cleanup. Include WAL packs, checkpoint packs, indexes, inherited lineage, and snapshots held by active readers. Protect reuse of the same content-addressed key while cleanup considers it. A TTL or an unreferenced-object scan alone races uploads and publication; a replaced checkpoint can also still be in use by a reader. Retaining published packs remains the default under D5. Do not combine this with broad history-pack garbage collection.

**Decision impact and validation.** A new decision must amend D3 for retention and replay behavior, D5/D6 for erasure and cleanup, and specify how D12's independent instances coordinate. Extend [delete lifecycle tests](../internal/jobs/delete_lifecycle_test.go), [fork boundary tests](../internal/repository/fork_watermark_test.go), and [multi-instance tests](../internal/repository/multi_instance_test.go). Add crash injection at every phase, concurrent fork/write/import/compaction, paused and expired workers, partial S3 deletion, versioned buckets, cross-namespace descendants, namespace creation races, name reuse, and replay after purge. Inspect every referencing table, result JSON, object version, and owned cache directory. Completion requires documented backup/offline-cache limits and real two-instance evidence.

### 2 Binary content in REST commits

**Current behavior.** [CommitFile](../internal/types/storage.go) contains only path and string content. [Validation and tree writing](../internal/repository/commit_tree.go) enforce 100 combined changes and 1 MiB of content, then hash string bytes into Git. Existing executable mode is preserved on an ordinary update; new files use 100644. [JSON decoding](../internal/api/write.go) has an independent 2 MiB request cap. Existing tests already cover preservation of untouched binary and executable files.

**Recommendation, updated 2026-10-05.** For the clarified hundreds-of-MiB workload, add streaming multipart to the existing commit route: a bounded JSON manifest followed by raw file parts, with one atomic Git publication. Preserve the existing small JSON path. Propose a configurable 512 MiB aggregate content limit, bounded active uploads, private disk staging, idle/total deadlines, and explicit mode strings 100644/100755. Omitted modes and symlink behavior retain the existing semantics. The full contract, proposed limits, recovery behavior, and delivery plan live in the [binary REST write proposal](binary-rest-writes.md).

Stage upload bytes before acquiring the repository cache lock. Prepare Git data and upload immutable objects before opening the final idempotency/publication transaction. Completed retries must win over stale-head checks; concurrent preparation on separate instances must still produce only one published result. This is a larger change than adding a base64 decoder. The issue's exact JSON encoding fields are deferred in favor of multipart for the clarified workload, so they must not be reported as implemented.

**Decisions and acceptance.** Define canonical multipart request digests independently of boundaries, part ordering, and temporary paths while preserving legacy JSON digest serialization. No resumable upload session is proposed: interrupted or uncertain attempts resend the complete keyed request. Extend [commit mode tests](../internal/repository/commit_modes_test.go), [interface retry tests](../internal/api/interface_test.go), [body-bound tests](../internal/api/write_bounds_test.go), and independent-instance races. Publish mixed files and executable modes, verify exact REST/Git bytes, recover after cache loss, and test cancellation, disk pressure, auth expiry/revocation, and lost responses. Measure 100 MiB files and 512 MiB aggregate commits before advertising capacity. This preserves D1/D2/D3 while changing preparation and resource ownership.

### 3 Postgres IAM token authentication

**Current behavior.** [Open](../internal/store/meta/postgres/store.go) creates a pgx pool from a static DSN. [Migrate](../internal/store/meta/postgres/migrate.go) independently uses database/sql through pgx. [Bootstrap](../internal/app/bootstrap.go), serving, token creation, and compaction call these entry points through different configuration paths; LoadDatabase currently reads only a DSN.

**Recommendation.** Introduce shared database connection configuration used by all commands. Add opt-in RDS IAM auth with an explicit database region override/fallback, endpoint and username validation, AWS default credential discovery, and TLS hostname/certificate verification. Keep password-based operation unchanged. Mint a token immediately before each new physical connection, not once at process startup or once per query.

The pinned pgx v5.7.6 supports both a [pool BeforeConnect hook](https://pkg.go.dev/github.com/jackc/pgx/v5@v5.7.6/pgxpool#Config) and [stdlib OptionBeforeConnect](https://pkg.go.dev/github.com/jackc/pgx/v5@v5.7.6/stdlib#OptionBeforeConnect), so migrations can use the same provider through an appropriate SQL connector. Add the AWS RDS auth module at a version compatible with the pinned SDK. Reject unsupported multi-host/socket configurations initially rather than minting for the wrong endpoint.

**Decisions and acceptance.** Choose the TLS/CA configuration and supported endpoint forms, plus cancellation and redaction for connection failures. Include AWS session credential refresh. RDS tokens expire after 15 minutes for new authentication; established sessions are unaffected, so a soak that only reuses existing pooled connections is insufficient. [AWS IAM documentation](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.html) supports that distinction. Test fresh connections after expiry and credential rotation, all three requested commands, and the token/compact commands too. Fake signers can test hook behavior locally; an actual IAM-enabled RDS role with no password is required to meet the issue's acceptance. Preserve D13's separate migration/runtime privileges.

#### Follow-up research: IAM and an existing database (2026-10-05)

The expanded requirement is to install Artifacts into a dedicated schema within an existing PostgreSQL database. Treat authentication and schema selection as independent configuration: named schemas must work with ordinary PostgreSQL credentials as well as RDS IAM. The initial proposal below assumes a new Artifacts installation in a new schema; relocating an existing installation from `public` requires a separate data-movement procedure. These are recommendations, not implemented settings or an adopted decision.

**Connection approach.** Retain the native pgx pool and use the official AWS SDK v2 RDS auth signer in both physical-connection hooks. Load AWS configuration and retain its refreshable credential provider once per database connector; assign each generated token directly to the connection config's password field. The [signer implementation](https://github.com/aws/aws-sdk-go-v2/blob/main/feature/rds/auth/connect.go) retrieves credentials and signs locally; it does not call an RDS token service. Credential discovery/refresh can still use network services. Per-connection signing therefore avoids an additional token cache and background refresh loop without implying a remote RDS API call per connection. Keep pool sizes bounded across instances and preserve ordinary session reuse.

The [AWS Advanced Go Wrapper](https://github.com/aws/aws-advanced-go-wrapper/blob/main/docs/user-guide/UsingTheGoWrapper.md) is a credible alternative when Aurora failover/topology plugins are required, but its public integration is through `database/sql/driver`. Replacing our native pgx interfaces or maintaining different database stacks would expand this request. Prefer the existing hooks for authentication alone. Token injection at process startup, a periodically rewritten environment variable, and the RDS Data API do not fit our existing connection and transaction model.

**Proxy compatibility.** AWS now documents [end-to-end IAM through RDS Proxy](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/rds-proxy-iam-setup.html), in addition to client-IAM with a password-backed proxy connection. Design the signer to use the endpoint actually selected for connection, including a proxy endpoint, and use the matching database or proxy resource in IAM policy. Direct RDS/Aurora writer endpoints should be the first verified path; proxy support requires its own acceptance run. A proxy is optional infrastructure, not a prerequisite for IAM.

Schema setup and migrations matter to proxy behavior: PostgreSQL `SET` and session advisory locks can cause [session pinning](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/rds-proxy-pinning.html). Our runtime advisory locks are transaction-scoped, which AWS exempts; the migration library uses a session lock. Native protocol prepared statements are not by themselves a reason to disable pgx caching: AWS supports [extended-protocol multiplexing](https://aws.amazon.com/blogs/database/amazon-rds-proxy-multiplexing-support-for-postgresql-extended-query-protocol/). Verify the actual startup schema parameters, permissions, and pinning metrics before making pooling claims. Use the direct writer endpoint for the initial migration job.

**Proposed configuration.** Keep `DATABASE_URL` for the existing database, username, endpoint, and TLS settings. Add `ARTIFACTS_DATABASE_AUTH=rds-iam`, an optional `ARTIFACTS_DATABASE_REGION` override with standard SDK region fallback, and `ARTIFACTS_DATABASE_SCHEMA=artifacts`. Schema selection is independent of IAM. An unset schema setting must preserve existing deployments and existing DSN behavior; new shared-database examples should explicitly select `artifacts`. Reject contradictory schema/search-path inputs instead of silently selecting one. Require verified TLS in IAM mode, following AWS's [verify-full example](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.Connecting.AWSCLI.PostgreSQL.html), and keep password-mode behavior unchanged. Do not fetch CA material dynamically during application startup.

One connection builder should parse pgx pool options once and pass only the resulting connection configuration to the SQL migration connector. Otherwise options such as `pool_max_conns`, which pgxpool consumes, can leak into PostgreSQL startup parameters when a DSN is reused through the SQL driver. All database commands must use the same auth/schema interpretation. The current migration driver also uses background contexts for some operations; implementation must explicitly bound connection and migration-lock waits and test cancellation rather than assuming a new context parameter alone solves it.

**Schema contract.** Treat the configured value as one identifier, quote it safely, and pin it for the lifetime of a pool. No per-request schema switching. A connection should search only the selected application schema and explicitly last-position `pg_temp`, letting PostgreSQL search `pg_catalog` implicitly first. Exclude `$user`, `public`, and other application schemas as fallback entries in named-schema mode. Verify the selected schema exists and is accessible before migration bookkeeping or serving. PostgreSQL documents both [search-path trust and implicit catalog lookup](https://www.postgresql.org/docs/current/ddl-schemas.html); simply appending our schema to an existing path is insufficient.

Keep the version table in the same schema, pass its schema explicitly to the migration driver, and qualify readiness/version checks. Merely configuring the driver's `SchemaName` does not relocate the unqualified DDL inside migrations; the connection's schema path must match too. Our pinned golang-migrate driver already incorporates database/schema/version-table into its migration lock key. Include the selected schema in our separate application idempotency and compaction lock namespaces so independent installations do not contend on matching logical IDs. Do not accept an unrelated migration ledger just because its version number happens to match; require an empty target or validate a recognizable Artifacts installation.

Allow an administrator to pre-create the schema owned by the migration role. Offer creation only as an explicit migration setup action, such as `migrate --create-schema`; missing schemas during normal serving should fail clearly. [Creating a schema requires database CREATE permission](https://www.postgresql.org/docs/current/sql-createschema.html), but migrating within an already-owned schema does not require owning the database. Never automatically create database roles, change other applications' grants, or revoke shared `public` privileges.

The migration identity owns only Artifacts objects; the serving/bootstrap identity receives schema USAGE, application-table DML, and read access to the version table, without schema CREATE/ownership. Configure [default privileges as the object-creating role](https://www.postgresql.org/docs/current/sql-alterdefaultprivileges.html) so upgrades preserve access. IAM users still need these SQL grants. Independent installations also need separate object-store prefixes and cache roots. A schema provides naming and permission boundaries, not separate database CPU, availability, or backup retention.

**Measured feasibility.** A temporary test against task-owned PostgreSQL 16 ran the existing `Migrate` twice with `search_path=artifacts,pg_temp`, using a login that owned the schema but lacked database CREATE permission. All 13 tables, including migration bookkeeping, appeared in `artifacts`. Existing `public.accounts` data and a distinct `public.schema_migrations` version remained unchanged. `Open`, `CheckSchema`, and an account write succeeded with a restricted runtime login; attempts to create a table or modify the neighboring application's data failed. A separate query confirmed that `search_path=missing,public` silently selects `public`. The experiment passed and removed its temporary test file and database container. This proves local feasibility of the existing SQL, not complete schema support, proxy compatibility, or AWS IAM authentication.

**Delivery and verification.** First unify connection configuration and implement schema isolation, keeping current migration SQL intact where possible. Then add IAM hooks, configuration, TLS validation, bounded errors, and credential redaction. Update deployment examples and append a decision amending D13 when implementation adopts the new setup contract. Extend verification with a populated neighboring application, two separate Artifacts schemas, conflicting table/version-table names, fresh pooled connections, repeated and concurrent migrations, missing/inaccessible schemas, runtime permission limits, and unchanged legacy installation behavior. Run the REST/Git workflow in the named schema. Real AWS acceptance must additionally force new connections after token expiry and workload-credential refresh, exercise failover/reconnection, and run migration/bootstrap/serving with their separate IAM identities. Preserve uncertain-commit recovery; connection recovery must not automatically replay arbitrary writes. Add proxy qualification separately if that deployment is needed. No IAM or schema-support implementation was added during this research.

**Implementation follow-up.** IAM connection hooks and dedicated-schema installation are now implemented under D15. See the [implementation and local evidence record](database-auth-and-schemas.md). The research above describes the proposal and its initial feasibility experiment; real AWS acceptance remains outstanding because no disposable IAM-enabled database was available.

### 4 S3 server-side encryption settings

**Current behavior.** All object uploads go through [s3.Put](../internal/store/object/s3/s3.go), which already supplies immutable-write preconditions and checksums. Copy currently reads the source and calls Put, so it will inherit encryption settings. No SSE fields exist in [configuration](../internal/config/config.go).

**Recommendation.** Add S3_SSE with empty, AES256, and aws:kms values, plus S3_SSE_KMS_KEY_ID valid only with aws:kms. Apply headers centrally while preserving checksums and conditional creation. Unset settings must leave request behavior unchanged. An explicit KMS key is advisable where bucket policy demands a particular key; AWS supports aws:kms without one using its managed key. See [SSE-KMS settings](https://docs.aws.amazon.com/AmazonS3/latest/userguide/specifying-kms-encryption.html).

**Decisions and acceptance.** Configuration governs new writes; it does not re-encrypt existing objects, and immutable Put may return success for matching data already present. Describe that boundary. Verify PutObject headers with a local test endpoint and preserve MinIO conformance. Then run real REST, Git, and compaction against a deny-unencrypted-put AWS bucket, with KMS upload and download permissions. [AWS permissions](https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingKMSEncryption.html) include GenerateDataKey for upload and Decrypt for download.

Defer the optional startup write probe to an explicit operator check with a unique object and cleanup. Keep readiness read-only as D13 specifies; adding a probe to every readiness request would create a new operational contract and recurring writes.

### 5 Rolling upgrades

**Current behavior.** [CheckSchema](../internal/store/meta/postgres/schema.go) requires exact equality with the newest embedded migration. [Deployment tests](../internal/store/meta/postgres/deployment_test.go) explicitly reject older, newer, missing, and dirty schemas. D12 permits only the same release; D13 and [deployment guidance](../docs/core/deployment.md) require a current schema and do not establish mixed-release support.

**Recommendation.** Support explicitly qualified adjacent releases using expand/contract migrations and a declared schema compatibility range. Keep exact migration ownership in one setup job and production serving in skip-migrations mode. Schema compatibility must include defaults, SQL queries, status values, idempotency serialization, jobs and leases, publication locks, and object formats. A numeric schema range alone is insufficient.

Use a preparatory release on the existing schema to introduce the compatibility contract and the checks required for the next expansion. An already deployed f59a410 binary cannot be made to accept a new schema by changing the next binary. Either qualify current/preparatory coexistence on schema 3 or schedule one transition with a full stop. Announce rolling support only from a tested pair onward. Never declare compatibility with arbitrary future migrations.

Expand with nullable/defaulted additions, deploy readers/writers that tolerate both representations, backfill separately, and contract only after old binaries and the rollback window are gone. Newly introduced token revocation, purge, or job states must not activate while an old server can ignore them. Feature activation is part of release compatibility.

**Decisions and acceptance.** Append an amendment to D12/D13 specifying the release window, rollback direction, dirty-schema behavior during a migration, and required full-stop upgrades. The migration framework's dirty marker can transiently make all old readiness checks fail; test and deliberately handle that interval without allowing arbitrary broken schemas. SQL DDL must use bounded lock waits, and risky backfills must be resumable. Do not run destructive down migrations against live newer writes as a rollback mechanism.

Extend [the container harness](../scripts/verify-container.py) to run two different immutable images with separate caches, continuous REST/Git writes, retries across versions, forks, compaction, deletion, revocation, and rollback after new-version writes. Keep an acknowledged-commit ledger and prove every acknowledged result reconstructs. Then exercise an owned Kubernetes rolling deployment, including readiness removal and termination timing. Specify both an allowed retry/error budget and retained service availability; an all-503 rollout technically fits the issue's error list but does not achieve its availability intent. Network disconnects still require idempotent reconciliation under D3.

### 6 Observability

**Current behavior.** There is no metrics route or request logging middleware. The production code's current slog use is a compaction error log in [maintenance](../internal/repository/maintenance.go). REST and Git use different routers beneath [the combined handler](../internal/app/app.go).

**Recommendation.** Start with Prometheus metrics and structured slog request logs. This fits an endpoint-based operator workflow with less initial scope than a complete tracing exporter. Define instruments at the HTTP, publication, cache, object-store, lock, and maintenance boundaries. Use normalized route/method/status and bounded outcome labels; repository IDs, tenant names, file paths, request IDs, and object keys must not become metric labels. This follows [Prometheus cardinality guidance](https://prometheus.io/docs/practices/instrumentation/).

Record REST/Git publication outcomes where Postgres commits, separately from HTTP outcomes and idempotent replays. Measure local mutex/flock and database wait time separately. Record full versus range reads, bytes, rebuilds, duration, and errors. Compute compaction backlog from a bounded or cached metadata query; the current candidate query is limited to eight rows and cannot represent the total backlog. Treat a shared-database backlog gauge as global rather than summing duplicate per-instance observations.

**Decisions and acceptance.** Choose an internal scrape listener/access policy, histogram buckets, log fields, and sampling. Validate or regenerate supplied X-Request-Id, return it, and carry valid traceparent context without claiming spans/export until implemented. Exclude bodies, authorization, signed tokens, DSNs, and unsanitized error strings that contain sensitive resource data. Response-writer instrumentation must preserve streaming, ResponseController unwrapping, cancellation, and idle deadlines.

Acceptance needs executable alert examples and fault injection: failed publication, slow storage, rebuild storms, lock contention, stalled compaction, and interrupted streams. Verify the exported signals alone distinguish those conditions. Add purge job age/failures when request 1 lands. Instrumentation should accompany the underlying features rather than become a final integration task.

### 7 Commit identity and conditional GET

**Current behavior.** [fileAt](../internal/api/content.go) resolves a commit and finds a blob but discards the commit identity. [handleFile](../internal/api/content_file.go) opens that blob and streams it with no validators. The range store may decode the full object during GetBlob, before Reader is called.

**Recommendation.** Resolve the commit and tree entry once into file metadata containing commit SHA, blob SHA, mode, and a deferred content opener. Set X-Artifacts-Commit, X-Artifacts-Blob, and a quoted blob ETag. Evaluate If-None-Match after authorization and path lookup but before opening/decoding blob content. Handle lists, weak validators for GET comparison, and the wildcard. Return validators and resolved identity on both 200 and 304; the same blob may be unchanged even though main now points to a different commit.

**Cache decision.** Use private, revalidated caching for moving refs. For a strictly validated full commit SHA, immutable needs an explicit freshness lifetime to be useful, and private caching is the appropriate baseline for authenticated content. See [HTTP caching](https://www.rfc-editor.org/rfc/rfc9111.html) and [immutable responses](https://www.rfc-editor.org/rfc/rfc8246.html). Recommend making positive SHA-cache freshness opt-in until the customer chooses its revocation/erasure window. Cached bytes can outlive server access revocation; server purge cannot invalidate the application's SHA cache. That dependency must be reconciled with request 1 before promising both behaviors.

**Acceptance.** Exercise ref movement during resolution, unchanged blob/new commit, all validator forms, missing paths, binary content, and denied/deleted access. Count blob fetches to establish whether 304 avoids payload work. Returning no HTTP body alone does not prove fewer object-store reads. Decide whether the same contract extends to raw/blob aliases or remains explicitly on /file.

### 8 Reading a folder in one request

**Current behavior.** [Tree reads](../internal/api/content_tree.go) expose direct entries without sizes. [EncodedObjectSize](../internal/store/packread/store.go) currently decodes the object, so adding a size column naively can fetch every file and even trigger the large-object disk fallback.

**Recommendation.** Add recursive tree listing with full repository-relative paths, clear file/directory/submodule types, deterministic order, bounds on entry count and traversal, and a resolved commit in every response. If paginated, bind continuation to that commit and requested subtree. Directory sizes should be omitted or null rather than invented. Investigate header-level object size lookup separately; measure its cost for delta objects before claiming metadata-only reads.

For this REST-only Node backend and small folders, prefer a bounded JSON batch read alongside recursive tree over implementing both batch and tar. Resolve a ref once per batch, return that commit with per-file path/blob/mode/size and base64 bytes, and make missing paths or exceeded limits fail the entire bounded response. Choose count, decoded-byte, encoded-response, depth, and deadline limits; initial proposals are 100 files and 8 MiB aggregate decoded bytes, subject to resource measurements. Twenty small files then fit tree plus one batch request. Binary content is lossless without archive extraction code.

**Tradeoffs and acceptance.** A POST batch route would be classified as a write by current [contentAuth](../internal/api/auth.go); explicitly classify this operation as a read and test read-only repository credentials. Symlinks should return their stored bytes/mode without following them; submodules need an explicit unsupported-entry result. If large streaming exports become required, tar is a separate option with traversal-safe member paths, entry/byte budgets, and partial-stream failure detection.

Test 20 mixed files, nested trees, empty folders, awkward names, special entries, moving refs, limits, and cancellation. Reuse the snapshot reader and measure requests/bytes/memory. Fewer client HTTP requests do not automatically mean fewer S3 ranges. A bounded buffered batch can ship before request 9; implement concurrent read ownership before unbounded streaming is considered.

### 9 Concurrent reads of one repository

**Current behavior.** [ReadContent](../internal/repository/read_remote.go) holds [an exclusive mutex and flock](../internal/repository/manager.go) through the callback, including HTTP streaming. It also repairs local indexes. Objects larger than 8 MiB invoke diskObjectReader, which can remove and rebuild the shared bare cache. A mechanical LOCK_EX-to-LOCK_SH change would allow those mutations to race.

**Recommendation.** Separate immutable snapshot reading from ownership of mutable cache files. A first delivery can prepare and bound small file content before the response stream, then release the repository lock; this removes slow-client lock ownership but still serializes resolution. Label it as an interim improvement.

For the full request, capture metadata/refs once, make verified indexes safely shareable through per-key coordination and atomic installation, and give each request its own pack reader/range state. Retain exclusive cache ownership for rebuild, receive, commit, and compaction. Large-object fallback needs a separate exclusive preparation phase plus a pinned cache generation or independent file handle/spool whose lifetime extends through consumption. Never attempt an in-place read-to-write lock upgrade inside the callback. Make waiting cancellable and bound active readers, decoded bytes, and scratch use.

Coordinate active snapshot lifetime with purge and orphan cleanup. A future collector must not delete a pack underneath a reader merely because the newest checkpoint replaced it. Preserve the same snapshot when switching to disk fallback; resolving main again could mix versions.

**Acceptance.** Extend [cross-manager/cache tests](../internal/repository/concurrency_integrity_test.go), [large-read tests](../internal/repository/large_read_test.go), [independent-cache tests](../internal/repository/multi_instance_test.go), and [stream timeout tests](../internal/api/content_test.go). Exercise missing/corrupt indexes, overlapping readers, a concurrent writer/eviction/compaction/purge, cancellation, and writer starvation. Measure warm and cold reads at stated concurrency and object sizes, including more than 8 MiB. Report wall time, CPU, memory, storage requests/bytes, and local materialization. Near-single-read elapsed time is a workload-dependent target, not a general throughput promise.

### 10 Path-scoped history

**Current behavior.** [readLog](../internal/api/content_log.go) walks commits in preorder, with limit/offset but no path filter and no hard maximum query limit. It does not currently reproduce all git log ordering and history simplification behavior.

**Recommendation.** Start with literal repository-relative file or directory paths, no glob/pathspec language and no rename following. Compare object identity and mode at that path between a commit and its parent; for directories compare subtree identity. This can skip unchanged trees without decoding file contents. Root commits compare against an empty tree. Filter before applying the existing offset/limit and preserve the unfiltered route's behavior.

**Decision.** Define merge selection and ordering explicitly. A first-parent comparison or all-parent predicate is a feasible product contract, but must not be advertised as exact git log -- path parity: Git applies [history simplification rules](https://git-scm.com/docs/git-log). If exact parity is required, use the pinned Git executable against a materialized snapshot with a literal pathspec, accepting the extra local reconstruction cost, or budget a larger walker implementation. Do not quietly change the query into first-parent-only history.

**Acceptance.** Cover root/add/edit/delete/mode changes, directories, merges, renames without follow, unusual literal names, missing paths, and pagination at a pinned commit. Add cancellation and a traversal budget with an explicit limit error, since a limit of 20 matches can still scan years of unchanged history. Benchmark sparse matches; a path filter is not necessarily a cheap read.

### 11 Fork targets

**Current behavior.** [Fork](../internal/jobs/fork.go) reuses the source namespace and [CreateSnapshot](../internal/store/meta/postgres/forks.go) captures parent sequence/refs under a database lock. [ForkRepoInput](../internal/types/repo.go) has neither a target namespace nor a commit field. Existing snapshot forks share whole packs; selecting fewer refs is already documented as not being a privacy filter.

**Recommendation.** Deliver namespace selection first. Add an optional target namespace that defaults to the source, require it to exist in the same account, keep control-token-only authorization, and use that namespace in the returned Git remote. Validate/fence the destination namespace within the creation transaction so namespace purge cannot race it. Cross-account forks remain out of scope.

For a selected commit, require a full lowercase SHA verified as a commit in the captured published snapshot. Capture the source publication, validate the object against that boundary, then create the child with a default branch and HEAD pointing to the requested SHA. Recheck source/destination lifecycle before finalization. Keep the captured publication boundary rather than trying to infer a unique publication sequence from commit ancestry; merges, multiple refs, and imported history defeat that shortcut.

**Decisions and acceptance.** Recommend that pinned mode creates only one branch and does not also copy current source refs. Define its interaction with default_branch_only and how an empty source fails. The inherited packs can contain objects later than the selected commit; pinning refs does not hide those objects from hash-based reads. A privacy-isolated historical copy requires an independent reachable-object export and a separate cost/design decision.

Extend [fork transaction tests](../internal/store/meta/postgres/v2_test.go) and [fork boundary tests](../internal/repository/fork_watermark_test.go) for old commits, inherited commits, source advancement/force-push, compaction, cache eviction, and both namespace and repository purge. Check cross-account denial and child independence through REST/Git. Document selected-commit behavior as an extension of D5, with purge dependency tracking from request 1.

### 12 Machine-readable contract

**Current behavior.** [Mounted REST routes](../internal/api/server.go) and hand-written wire types are the contract sources. There is no OpenAPI document or route-parity check. Some collections initialize an empty slice, while others return nil and serialize to null; repositories, credentials, and log use different pagination models.

**Recommendation.** Check in an OpenAPI 3.1 contract for the current REST surface, including aliases, errors, binary response media types, authentication alternatives and route-specific credential permissions, idempotency headers, and limits. [OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html) is a suitable explicit target; pin validator/generator versions. Keep the Git wire protocol outside generated REST clients, with its endpoints documented separately or explicitly excluded from parity checks. Include health endpoints in the documented operational surface.

Use chi route traversal for method/path parity and representative HTTP request/response tests for actual schemas and headers. Route parity alone cannot detect a wrong status code, nullable collection, or authorization rule. Generate and compile a small TypeScript client fixture in CI and exercise binary upload/download and error decoding; shipping an official SDK is unnecessary.

**Decisions and acceptance.** Normalize successful empty collections to [] with targeted regressions; do not alter intentionally nullable fields. Choose opaque cursor pagination with a bounded limit for new list operations, binding content cursors to their snapshot. Existing cursor strings sometimes encode offsets, so their existence alone does not establish stable pagination under concurrent mutation. Keep legacy pagination unchanged unless a separate migration is requested. Extend the contract in each feature PR. A deliberately undocumented route and an invalid response example should both fail CI.

### 13 Control-token lifecycle

**Current behavior.** [API tokens](../internal/types/token.go) have ID/hash/account/created time, and the [metadata interface](../internal/store/meta/store.go) only supports create and hash lookup. [Authorization](../internal/auth/issuer.go) checks account/hash but no expiry or revocation. [Bootstrap](../internal/store/meta/postgres/bootstrap.go) reuses a matching hash and is repeated by serving when ARTIFACTS_API_TOKEN is supplied. The legacy bare token command also prints ARTIFACTS_API_TOKEN; changing that behavior needs an explicit compatibility choice.

**Recommendation.** Add nullable expires_at and revoked_at, preserving existing tokens as non-expiring. Implement account-scoped CLI list and revoke plus an optional expiry on create. Lists return safe identifiers and dates only; new list/revoke commands must never print secrets or hashes. Preserve the existing create output contract. Use database configuration only for administrative token commands and retain role separation from D13.

Retain a revoked hash so repeated bootstrap or server startup cannot silently reactivate it. EnsureAPIToken should reject an expired/revoked injected credential with a redacted, actionable error; bootstrap with a new secret adds a new active credential. Every request must check current state, as it does for repository tokens. Keep generated token expiry in the database rather than changing the bearer format.

**Decisions and acceptance.** Choose TTL versus absolute expiry flags, maximum lifetime, and treatment of revoking the last active token. Recommend the rotation sequence create/bootstrap new, verify it, switch the application, then revoke old. Direct database CLI access provides recovery without adding a public token-management API. Append a D13 amendment for bootstrap's new handling of revoked/expired credentials. Test account isolation, exact expiry boundary, repeated revoke, two-instance revocation, and restart/bootstrap with a revoked secret. During mixed-version operation, expiry/revocation cannot be claimed until every serving binary enforces it.

## Decisions to settle before implementation

These are proposed choices, not numbered decisions. The next register entry is D14 at this revision; assign IDs only when recording an adopted choice, then leave that entry immutable.

| Choice | Recommended starting point | Existing decision affected |
| --- | --- | --- |
| Erasure completion | Explicit asynchronous purge; refuse unpurged fork dependencies; define durable storage, active caches, offline copies, and backup boundaries separately | Amend D5/D6; coordinate under D12 |
| Replay after erasure and retention | Remove result content, use bounded opaque suppression for purged create requests; finite retention opt-in until migration policy is chosen | Amend D3 |
| Purge jobs and garbage collection | Durable operation state, leases plus fencing, tracked uploads/readers, bounded work; retain published history by default | D1/D5/D6/D12 |
| Rolling release compatibility | Qualified adjacent pairs, expand/contract, a preparatory release, explicit rollback window and activation gates | Amend D12/D13 |
| Large binary REST writes | Streaming multipart, bounded disk staging, preparation before a short transactional publication, canonical retry digest; whole-request retries initially | Preserve D1/D2/D3/D12; extend write/resource contract |
| SHA caching versus access revocation | Private conditional caching by default; positive immutable freshness only with an agreed stale-access window | Clarify D4 and purge contract |
| Folder representation | Bounded JSON batch plus recursive tree for the stated small-folder use case | Extend read contract |
| Pinned fork meaning | One branch at the requested commit within a captured publication; shared packs are not a privacy boundary | Extend D5 |
| Path history semantics | Literal paths, no rename following; explicitly choose merge rules or budget exact Git parity | Extend read contract |
| Token revocation and provisioning | Persistent revocation, no bootstrap resurrection, rotation with overlap | Amend D13 |
| Contract and operations | OpenAPI maintained with routes, cursor pagination for new lists, Prometheus with bounded labels | New working/API convention if adopted |

The highest-value customer inputs are the required erasure completion boundary and retention window, whether one initial full-stop upgrade is acceptable, and whether batch limits cover their actual folder sizes. The recommendations above let unrelated work proceed while those are resolved.

## Delivery sequence

| Stage | Concrete deliveries | Exit condition |
| --- | --- | --- |
| A Contracts and foundations | Adopt erasure/replay and release compatibility decisions; baseline OpenAPI/route parity; define owned AWS acceptance environment; instrument publication/storage/request basics | Precise acceptance and migration plan for changes that affect durability/authentication; current contract captured |
| B Integration features | Separate PRs for 4 SSE, 3 IAM across all connection paths, and 7 read metadata/conditional GET; staged delivery for 2 large binary commits; finish remaining baseline metrics | Customer can publish mixed files at verified limits and operate on its required AWS auth/encryption setup, with real AWS evidence |
| C Purge lifecycle | Operation/lease and typed ownership schema; in-flight work tracking; resumable repository purge; namespace purge/dependency order; cache/version cleanup; replay retention; orphan cleanup | Every part of request 1 passes crash/race/storage inspection; content erasure limits are explicit |
| D Remaining application features | 13 token lifecycle; 9 concurrent snapshot reader; 8 recursive tree and batch; 10 path log; 11 namespace then pinned forks | Requested workflows pass end-to-end acceptance and measured read targets; authorization remains consistent |
| E Release qualification | Complete mixed-release, rollback, migration-lock/readiness, and Kubernetes rollout testing; reconcile all OpenAPI and alert coverage | Request 5 can be advertised for named tested release pairs; all 13 issue criteria have evidence |

Stage C's design is the critical path and should start during A; B changes need not wait for every purge implementation detail. Binary uploads must honor purge fencing when that lifecycle lands; safe orphan deletion remains part of C. Observability and OpenAPI are maintained in each stage, not left incomplete until E. If rolling upgrades are required for the schema changes in C/D, deploy the preparatory release and qualify the first pair before those migrations. Otherwise explicitly accept a full-stop transition and keep request 5 open until E. S3/IAM/binary/identity changes can be delivered without a schema migration.

Dependencies that should determine PR order:

- Database auth configuration precedes further token CLI connection changes.
- Purge/read ownership and upload tracking precede orphan deletion and the fully concurrent range reader.
- Namespace purge fencing precedes cross-namespace fork creation; the latter must honor it from its first release.
- Read metadata resolution is reusable for conditional GET, recursive listing, batch responses, and snapshot pinning.
- Instrumentation precedes performance claims, while the OpenAPI baseline precedes new endpoint contracts.
- Every serving binary must understand lifecycle and credential states before the deployment enables operations relying on them.

Start implementation with the OpenAPI baseline and SSE change while the purge contract is being finalized, then binary commits and shared database authentication. Each is reviewable in isolation. Split request 1 into implementation tickets because it includes purge, namespace coordination, retention, and orphan cleanup; keep issue 1 as the adoption checklist. Splitting should happen only when moving into implementation, rather than publishing speculative commitments during this assessment.

## Verification and cost accounting

Every code delivery must pass make verify and update its current contract docs, callers, examples, and migration guidance. Public REST/Git, storage, authentication, and recovery changes also require the [verify-artifacts skill](../.agents/skills/verify-artifacts/SKILL.md), its preflight, and a real REST → Git → REST run on owned resources. Site changes require npm run check in docs-site. After an authorized push, inspect CI for that exact commit under D11.

Existing tests provide useful starting points but are not proof of the proposed features. Preserve the independent-cache snapshot, conflicting writer, idempotency, fork watermark, corruption, large-object fallback, credential, and shutdown regressions. Add targeted failures rather than tests that merely restate implementation. Reuse the container harness for a combined acceptance scenario: migrate/bootstrap, write mixed content, pin and batch-read it, fork it, rotate credentials, roll a qualified release pair, remove caches, reconstruct, and finally purge child then parent/namespace. Add independent fixtures for blocked purge and interrupted jobs.

AWS acceptance requires task-owned RDS/IAM and S3/KMS resources, credentials injected outside arguments/output, and recorded cleanup. MinIO does not prove AWS IAM authentication, KMS enforcement, or AWS versioned-erasure behavior. Kubernetes rolling behavior also needs its own owned environment. Provisioning and running these environments are future implementation work; none were created for this assessment.

| Change | Expected cost effect to verify |
| --- | --- |
| Binary writes | Raw multipart avoids base64 expansion; bounded ingress uses disk staging; full local Git cache, loose objects, generated pack/index, and S3 staging can amplify disk use; measure duplicate preparation on concurrent retries |
| IAM/SSE | Token generation per physical DB connection; SSE headers leave application object bytes unchanged; KMS adds provider operations |
| Purge/GC | Metadata scans, paginated key/version lists, delete requests, cache cleanup; no need to download all file contents just to delete keys |
| Conditional GET | No response body on 304; blob payload work avoided only if metadata lookup stays separate |
| Recursive tree/batch | Fewer client requests; size lookup/range layout may still fetch substantial storage bytes; batch buffering needs an aggregate bound |
| Concurrent reads | Less serialization; potentially more simultaneous range requests and memory; large fallback may still materialize a full repository |
| Path history | Work depends on commits traversed and changed trees; sparse path history can be expensive |
| Forks | Namespace/pinned-ref changes can remain metadata-based; privacy-isolated copying would transfer and materialize objects |
| Observability/rolling | Bounded instrumentation overhead; overlapping releases temporarily duplicate cache and DB connection use |

For read performance, use reproducible warm/cold fixtures with many small files, an object above the current 8 MiB fallback threshold, long publication history, and fork lineage. Record client latency, storage requests and bytes, CPU, peak memory, and local materialization before and after. Select numerical targets from those fixtures rather than treating the issue's near-single-read latency as already established.

## Assessment validation

The issue body and empty comment list were fetched from GitHub. Implementation paths, migration tables, mounted routes, relevant regression tests, verification configuration, and current decision/docs contracts were inspected at the stated revision. AWS, pgx, HTTP, Git, Prometheus, and OpenAPI references above were checked against their primary documentation.

This assessment and the later binary-write proposal add only review documents and the source snapshot under reviews, which is outside the documentation site's content loader. Validation checks source fidelity, local links, all 13 request sections, whitespace, and the final diff. Runtime tests, make verify, live acceptance, AWS deployment, and performance benchmarks are not represented as run or passed by this assessment.
