# Production deployment verification — 2026-09-20

The production image passed the container acceptance run on Linux/arm64. The real REST → Git → REST harness reports classification `none`, with repository cleanup confirmed. See the [core report](evidence/production-deployment/core/report.md), [container assertions](evidence/production-deployment/checks.json), [image identity](evidence/production-deployment/image.json), and [working-tree source hashes](evidence/production-deployment/source.json).

The run started from an empty Postgres database, rejected bootstrap before migration, ran migrations twice, and bootstrapped the same account/token concurrently. Exactly one token remained. Rebinding it to another account failed without leaving that account behind, and setup output contained no credentials.

Two instances used the built image as UID/GID 10001 with read-only root filesystems, separate ephemeral caches, shared MinIO, and a database role without schema-creation permission. The checks exercised dependency outages, real REST/Git interoperability across nodes, conflicting writes, idempotent retries, clean SIGTERM exits, and reconstruction after removing a container and its cache. The run passed 38 functional/evidence assertions plus owned-resource cleanup. All owned containers, the network, and scratch directories were removed; redacted evidence remains.

`make verify` passed formatting/imports, vet, lint, race tests, coverage thresholds, and Go file-size checks. Regression tests additionally cover secret-file validation, schema mismatch/dirty states, health endpoint behavior, and both successful request draining and forced cancellation at the shutdown deadline. `npm run check` passed in `docs-site/`.

Reproduce from the repository root:

```bash
docker build -t artifacts:production-check .
python3 scripts/verify-container.py --image artifacts:production-check --evidence /tmp/artifacts-container-evidence
```

The CI workflow now builds the actual image and runs this acceptance test, preserving its evidence. That workflow has not been pushed or run remotely for this change. Kubernetes/ECS deployment guidance has not been exercised on either platform. No registry image was published, and amd64 execution is not established by this local arm64 run. Mixed-release upgrades remain unsupported.

Publication storage behavior is unchanged: this work does not reduce transferred bytes or local materialization. Readiness adds a schema query and object-existence request per probe (an S3 HEAD on this backend, with SDK retries bounded by the probe context). It checks readability, not write permissions. Graceful draining does not guarantee completion of requests that exceed the configured deadline, and an uncertain response still requires reconciliation.
