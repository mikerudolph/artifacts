# Database authentication and schema installation

Implementation record for the 2026-10-05 working tree. The architectural decision is [D15](../AGENTS.md#d15-2026-10-05-configure-database-identity-and-schema-consistently); current operator instructions live in [deployment](../docs/core/deployment.md) and [configuration](../docs/core/configuration.md). This record does not establish AWS qualification.

## Implemented behavior

All database commands use one configuration builder. Native pgx serving connections and the SQL migration connector share endpoint, TLS, schema, and authentication settings. Pool-only DSN options are consumed before migration connections are opened. With `ARTIFACTS_DATABASE_AUTH=rds-iam`, the official AWS SDK signer retrieves credentials from the default provider chain for each physical connection and assigns the token directly to that connection's password field. Existing sessions use normal pooling. IAM requires a single DNS endpoint, username, verified TLS, and no static database password. Setup errors redact connection strings, passwords, token contents, and provider diagnostics.

`ARTIFACTS_DATABASE_SCHEMA=artifacts` installs into a dedicated namespace in an existing database. The fixed search path excludes neighboring application schemas; migration history and readiness refer to the same schema. Both idempotency and compaction locks include the selected schema. Migrations reject unrelated objects and inconsistent migration history. Existing deployments with no explicit schema keep their DSN behavior.

The schema can be pre-provisioned for a migration role without database `CREATE`. Alternatively, `migrate --create-schema` explicitly creates it when the caller has permission. Serving does not create missing schemas. Runtime and bootstrap operate with schema usage and data permissions, without schema ownership. Migration deadlines cover connection setup, lock waits, and migration execution; cancellation closes the migration connection even where the migration driver uses background contexts. Inspect a dirty or interrupted migration before retrying; no arbitrary transaction replay is added.

## Local evidence

Tests cover named-schema installation and upgrades, concurrent migration jobs, missing/inaccessible schemas, conflicting search paths, neighboring table/ledger sentinels, restricted runtime grants, and schema-independent idempotency/compaction. CLI tests exercise token creation and compaction with both default and named schemas. Local TLS protocol tests observe signed passwords on each fresh native-pool and SQL connection; signer tests exercise renewed credentials, endpoint/region/session fields, and redaction. Handshake and migration-lock tests exercise deadlines.

The [first container run](evidence/database-2026-10-05/checks.json) passed 44 assertions plus cleanup using the source/image identity in that directory. It ran doctor and real REST → Git → REST against two non-root containers with independent caches and a shared named schema, then checked retries, readiness recovery, cold restart, and unchanged neighboring application data. That image preceded the final timeout bound on schema validation. Final acceptance evidence is recorded separately so the earlier run remains attributable to its tested revision.

The [final container run](evidence/database-2026-10-05-final/checks.json) passed all 45 acceptance and cleanup assertions. Its [source hashes](evidence/database-2026-10-05-final/source.json) match the final runtime source, and the [core report](evidence/database-2026-10-05-final/core/report.md) has classification `none`. The image is recorded in [image.json](evidence/database-2026-10-05-final/image.json); logs include the build, documentation checks, and skill validation.

The final coverage run exposed an existing unsynchronized checkpoint map in the repository test double. The [race report](evidence/database-2026-10-05-final/make-verify-checkpoint-race.log) is preserved. Protecting that map with a read/write mutex fixes the fixture without changing production behavior or suppressing the race detector. Twenty consecutive targeted race runs passed after the fix.

The subsequent full [`make verify`](evidence/database-2026-10-05-final/make-verify.log) passed formatting/imports, vet, lint, race tests, coverage thresholds, and file-size gates, with 89.9% total coverage. The [documentation checks](evidence/database-2026-10-05-final/docs-check.log) and [skill validation](evidence/database-2026-10-05-final/skill-validation.log) also passed. No push was requested; remote CI was not run.

## Limits

No IAM-enabled RDS/Aurora database was available. AWS login authorization, workload-role renewal in a running service, direct-writer failover, and RDS Proxy behavior are unverified. The [verification skill](../.agents/skills/verify-artifacts/references/features/database-deployment.md) specifies those additional acceptance steps without treating local signing as an AWS pass.

This installs into a new dedicated schema; it does not relocate an existing Artifacts installation from `public`. Separate schemas still share database compute, availability, and backup boundaries. Independent installations need distinct object-storage prefixes and cache roots. The change adds no object transfers or local Git materialization to publication requests; storage layout and byte-transfer behavior are unchanged, and no throughput claim is made.
