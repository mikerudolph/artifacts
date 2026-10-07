# Encrypted object storage verification

Read `docs/core/configuration.md` and the S3 encryption section of `docs/core/deployment.md` for the current contract. `S3_SSE` affects new uploads; matching immutable objects can be reused without re-encryption. Retained history can still depend on older keys.

## Local acceptance

Run `make verify` for settings validation, exact encryption headers on uploads and retries, encrypted copies, unchanged conditional writes/checksums, policy rejection without downgrade, and absence of write-only encryption headers on reads and deletes.

Build the current image and run each mode with its own persistent evidence directory:

```bash
docker build -t artifacts-encryption-verify:local .
python3 scripts/verify-container.py --image artifacts-encryption-verify:local \
  --s3-sse AES256 --evidence /tmp/artifacts-encryption-aes256
python3 scripts/verify-container.py --image artifacts-encryption-verify:local \
  --s3-sse aws:kms --database-schema artifacts --evidence /tmp/artifacts-encryption-kms
```

The driver owns its containers and local MinIO key. It runs doctor and real REST → Git → REST, compacts history, reconstructs a cache after restart, and checks encryption metadata on stored objects. Preserve `encryption.json`, `checks.json`, source/image identity, and core reports. Confirm cleanup and the secret scan. The default run without `--s3-sse` checks compatibility without explicit encryption headers.

The local key implements MinIO encryption, not AWS KMS. HTTP policy fixtures prove request behavior, not AWS authorization. Report those distinctions even when all local checks pass.

## AWS acceptance when an authorized target exists

Use an authorized disposable bucket or isolated prefix, workload identity, and KMS key. Do not create billable infrastructure or edit shared policies just to fill an evidence gap. Without such a target, complete local checks and report AWS KMS enforcement as unverified.

1. Configure a bucket policy requiring the chosen encryption header and, for customer-managed KMS, the exact key ARN. Verify a write with missing headers or a wrong key is denied using a scoped test identity.
2. Run doctor and the core workflow with the configured workload identity. Inspect object encryption metadata and the key identifier for packs and indexes. Keep signed requests and credentials out of evidence.
3. Compact, discard only the owned instance's disposable cache, and verify REST/Git history reads and keyed retry recovery. Inspect checkpoint encryption too.
4. Test denied KMS access with a separate scoped identity or pre-authorized policy change. Confirm the publication fails without advancing the ref and does not retry with encryption disabled. Readiness alone cannot establish this.
5. If changing keys for an existing test installation, retain decrypt access to earlier keys and verify both old and new history. Do not interpret the configuration change as historical re-encryption.

Record the tested mode, key class, object metadata results, failed-permission behavior, and cleanup. Remove only owned test data, identities, and pre-authorized policy changes. Do not make performance or KMS-cost claims from small fixtures.
