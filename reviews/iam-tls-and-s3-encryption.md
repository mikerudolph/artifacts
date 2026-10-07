# IAM TLS options and S3 encryption

Implemented 2026-10-06 on top of `dce97a81b8d0b16be05d5ea946d280367c69dc35`, following the [IAM TLS follow-up](https://github.com/mikerudolph/artifacts/issues/1#issuecomment-6018513376) and request 4 in the [integration issue](https://github.com/mikerudolph/artifacts/issues/1). Current configuration and deployment guidance live in [configuration](../docs/core/configuration.md) and [deployment](../docs/core/deployment.md).

## Implementation

IAM accepts required TLS with the operator's selected verification level. D16 amends D15 without changing the earlier decision. Signing, connection renewal, endpoint restrictions, schema isolation, and transaction behavior are unchanged. Both the native pgx pool and SQL migration connector share validation. One process-wide warning describes weaker server verification without printing connection details.

pgx can upgrade `prefer` under direct TLS negotiation, and `sslrootcert=system` can upgrade modes to `verify-full`. To honor the explicit mode allowlist, validation asks pgx to parse the same DSN/environment/service settings with those two upgrades neutralized. This policy-only parse does not replace or alter the actual connection's TLS configuration, trust roots, verification callbacks, or negotiation. Tests cover both upgraded modes and environment/service-file selection.

S3 configuration validates `AES256` and `aws:kms`, with a key identifier permitted only for KMS. The immutable object store sets encryption fields on its shared `PutObject` request, so copies and all publication/compaction object types inherit the setting. Reads and deletes do not carry write-only encryption headers. Existing immutable objects can still be reused; the setting does not rewrite history or rotate keys. Readiness remains read-only.

No schema migration is needed. S3 encryption changes headers without changing application object bytes, local staging, or the existing upload/read request pattern. This is an implementation observation, not a measured performance improvement. AWS KMS can add provider operations, latency, quotas, and cost; none was measured here.

## Verification

- `env GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 make verify` passed: formatting/imports, vet, lint, race tests, coverage thresholds, and file-size limits. Total coverage was **90.0% (4268/4740)**. The environment isolates test Git configuration without modifying developer settings.
- IAM regression tests exercise signed tokens over TLS for both connectors; certificate/hostname rejection; plaintext refusal; TLS mode selection through URI, keyword, environment, and service configurations; credential renewal; one-time warnings and redaction. Every database command reaches TLS authentication with `require` and no CA. That command fixture deliberately rejects authentication after capturing the signed token, so it does not establish command completion on RDS.
- S3 HTTP contract tests cover no configured encryption, SSE-S3, KMS with and without a key, exact headers on retries/copies, unchanged immutable/checksum semantics, byte-range reads, and denial when policy headers do not match.
- [KMS container evidence](evidence/iam-sse-2026-10-06-kms-final/checks.json): **79 assertions**, including cleanup, with named-schema isolation. [Encryption metadata](evidence/iam-sse-2026-10-06-kms-final/encryption.json) checks all **16 objects** against the requested mode and MinIO's canonical identifier for the test key.
- [SSE-S3 container evidence](evidence/iam-sse-2026-10-06-aes256/checks.json): **57 assertions**, including cleanup, using the default schema. [Encryption metadata](evidence/iam-sse-2026-10-06-aes256/encryption.json) checks all **16 objects**.
- Both container runs include doctor, real REST → Git → REST, two independent caches, compaction, cache loss/restart, keyed retries, readiness recovery, and owned-resource cleanup. Source hashes match the implementation under test; core evidence hashes and sizes were checked.
- `npm run check` in `docs-site` passed. The verification skill's validator and local reference links passed; its database and S3 references describe the new acceptance paths.

## Failures retained during verification

The [first KMS run](evidence/iam-sse-2026-10-06-kms/core/report.md) completed encrypted REST writes and Git clone, then inherited the developer's interactive Git signing configuration. The local 1Password signing process failed before push; the harness recorded a product classification, but the Git log identifies the client-environment cause. Cleanup passed. The driver now isolates its Git configuration. The initial unisolated full-suite run hit the same harness signing failure; the isolated full suite passed.

The [second](evidence/iam-sse-2026-10-06-kms-pass2/checks.json) and [third](evidence/iam-sse-2026-10-06-kms-pass3/checks.json) KMS runs completed functional checks but failed an assertion expecting the bare key name. The third run preserved [returned metadata](evidence/iam-sse-2026-10-06-kms-pass3/encryption.json), showing `arn:aws:kms:artifacts-test`. The final driver checks that exact MinIO representation. Cleanup passed for both failed assertion runs; their reports remain unchanged.

## Limits

No AWS infrastructure was provisioned or used. Local MinIO uses an ephemeral test encryption key, not AWS KMS. Actual RDS IAM login, workload credential renewal, failover/Proxy behavior, and AWS S3/KMS authorization or bucket-policy enforcement remain unverified. The verification skill includes separate acceptance steps for an authorized disposable AWS target. No automatic startup write probe, historical re-encryption, bucket/key provisioning, or encryption-policy downgrade was added.
