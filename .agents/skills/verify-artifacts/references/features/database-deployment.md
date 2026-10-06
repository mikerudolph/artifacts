# Database deployment verification

Read `docs/core/configuration.md` and `docs/core/deployment.md` for the current contract. Database credentials and schema selection apply to every command. A named schema is a new installation boundary, not a relocation tool for existing `public` data. Changes to this contract must preserve the append-only decision register, including D15.

## Local acceptance

Run `make verify` for configuration rejection, both physical-connection signing hooks over local TLS, credential refresh, cancellation during handshake/lock waits, concurrent migrations, upgrades, and separate schema permissions. The fake PostgreSQL authentication endpoint checks signed credentials on the wire; it does not validate AWS authorization.

Build the current image and use a fresh persistent evidence directory:

```bash
docker build -t artifacts-database-verify:local .
python3 scripts/verify-container.py \
  --image artifacts-database-verify:local \
  --database-schema artifacts \
  --evidence /tmp/artifacts-database-evidence
```

The driver owns its Postgres, MinIO, network, servers, and scratch state. It provisions `public.accounts` and a conflicting `public.schema_migrations` sentinel, installs into the chosen schema using a migration role without database `CREATE`, checks restricted runtime permissions, and runs doctor followed by the real REST → Git → REST drive. It also checks two independent caches, readiness recovery, durable retries, and process restart. Preserve `checks.json`, source/image identity, doctor output, and core evidence. Check the cleanup assertion and secret scan; a successful core drive alone does not prove cleanup or schema isolation.

Run without `--database-schema` when default-schema compatibility needs acceptance coverage. Use a separate evidence directory. The normal package suite already exercises unchanged default-schema deployments.

## AWS acceptance when a disposable target exists

Do not create billable AWS infrastructure merely to fill an evidence gap. If no authorized IAM-enabled RDS/Aurora target exists, complete local verification and explicitly report AWS login, workload credential renewal, failover, and proxy behavior as unverified.

For an authorized target, record the region, endpoint class, database engine/version, release/image, selected schema, and migration/runtime usernames. Keep profiles, connection strings, tokens, access keys, and signed query strings out of evidence. Use separate migration and runtime workload identities, a pre-provisioned CA bundle, `sslmode=verify-full`, and the direct writer endpoint first.

1. Migrate twice, bootstrap twice with the same injected control token, and start serving with `ARTIFACTS_SKIP_MIGRATIONS=true`. Confirm all metadata and migration history remain in the selected schema and neighboring objects are unchanged.
2. Run doctor and `verify-core` with fresh evidence. Exercise a successful publication and retry its original idempotency key after reconnection.
3. Keep the same serving process alive longer than the IAM login token's validity. Force fresh connections by expiring a short-lived pool or terminating only the owned test role's idle sessions. A process restart alone does not test renewal in a running connector. Prove a new backend session and successful REST/Git operations without process restart.
4. Repeat after workload credentials actually rotate or expire and are renewed. Record timestamps and backend-session changes, never token contents. A static developer key cannot establish workload-role renewal.
5. Exercise a scoped network interruption or an explicitly authorized test failover. Confirm eventual new connections and readiness, then reconcile uncertain publications with their original keys. Do not interpret a transport error as proof of rollback or expect automatic replay of arbitrary transactions.
6. Test denied IAM permission, wrong region/user, untrusted CA, and hostname mismatch with task-owned identities/configuration. Confirm bounded failure and redacted output. Do not alter shared policies or terminate unrelated sessions.

Proxy acceptance is separate: use the proxy endpoint for signing, the corresponding resource policy, and the intended proxy authentication mode. Repeat connection renewal and REST/Git checks; measure session pinning before making pooling claims. Do not infer proxy acceptance from a direct RDS pass.

Clean up only owned repositories, schemas, sessions, processes, and temporary policy changes within the authorized target. Preserve redacted evidence and list any cleanup that could not be confirmed.
