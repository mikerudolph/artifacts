# Independent serving instances

Recorded 2026-09-20 against the working tree based on `80ac7f264d61c9de817f1f36a19b1f440a3413c2`. The topology decision is [D12](../AGENTS.md#d12-2026-09-20-coordinate-independent-serving-instances-through-postgres). Current deployment instructions live in [configuration](../docs/core/configuration.md#multiple-serving-instances).

## Change and concurrency boundary

A cache rebuild previously read repository sequence and current refs separately. A publication from another cache could advance refs between those reads, leaving reconstruction with refs whose objects had not been downloaded. Range reads attempted to avoid this by reading the repository before and after refs, but could exhaust three attempts under continuous publication.

Repository metadata and refs now come from one SQL statement, including within the existing transaction used by idempotent commits. The captured sequence bounds immutable pack history. A newer checkpoint is skipped when it exceeds that boundary; immutable fork lineage and retained WAL packs remain available. Both range reads and their large-object disk fallback retain the captured refs.

Postgres still locks and validates each publication atomically. Independent writers can prepare work simultaneously, but a competing repository sequence or old ref rejects publication. There is no automatic merge, and concurrent changes to different branches can conflict. Git success remains buffered until publication commits; a publication conflict returns HTTP 409 rather than 500.

Compaction takes a nonblocking per-repository advisory transaction lock in Postgres. A second compactor skips overlapping work. Publication does not acquire that lock. The compaction transaction retains a database connection through object work, which operators must include when sizing pools. Existing conditional checkpoint replacement prevents older work from regressing the checkpoint.

## Verification

`make verify` passed: formatting/imports, vet, lint, race tests, coverage gates, and file-size checks. Total coverage was 89.3% (3528/3951 statements). `npm run check` in `docs-site/` passed.

Regression tests cover:

- Captured refs remaining usable while another independent cache publishes and compacts during a disk or remote-index download; the next read observes the new publication.
- Simultaneous prepared REST commits against one repository, with exactly one publication, a recoverable loser, and no extra event.
- Concurrent identical idempotency keys across independent caches and rejection of a changed payload.
- Atomic repository/ref snapshot reads during repeated publications, including snapshots inside an existing transaction.
- Compaction lock exclusion, release after failure, cancellation, and publication while maintenance holds its advisory lock.
- Git publication conflict versus storage-failure HTTP status.

The [live reproduction](evidence/multi-instance/reproduce.py) built two authenticated server processes, each with its own cache, against a task-owned Postgres container and an S3-compatible MinIO container. A round-robin HTTP proxy exercised the full harness without session affinity. Direct requests then exercised known instance-to-instance transitions.

The [core harness](evidence/multi-instance/core/report.md) classified the run as `none` (success), with all eight assertions passing and repository cleanup confirmed. [Additional evidence](evidence/multi-instance/checks.json) records 21 passing checks including cleanup: cross-instance REST/Git visibility, competing REST and Git writes, idempotency, forks, concurrent compaction, acknowledged-data availability after stopping one server, cold restart after removing that server's cache, credential revocation, and parent deletion without losing fork content. [Routing evidence](evidence/multi-instance/routing.log) records request placement without headers or credentials. All owned processes, containers, and scratch directories were removed.

Two earlier evidence runs were rejected by the harness infrastructure: an overly strict assertion expected round-robin Git POSTs to reach both servers, and a subsequent run attempted preflight before the proxy was listening. The final reproduction checks actual routing and waits for proxy readiness. Neither failed run is represented as a successful product verification.

## Storage and operational effects

Immutable pack format and publication uploads are unchanged. The metadata snapshot replaces separate repository/ref queries, not object transfers. Each serving instance retains independent index and Git caches, so additional instances can duplicate S3 reads, downloaded bytes, and disk materialization. Git serving and writes still require full local repository reconstruction; small content reads retain indexed range access and large-object fallback. Compaction exclusion avoids simultaneous duplicate work but does not prevent sequential explicit compactions.

The live fixture did not measure throughput, CPU, transferred bytes, or S3 request counts. It establishes concurrency behavior, not a performance improvement.

The verified topology uses the same release, one shared database writer, and one shared bucket/prefix. It does not establish mixed-release upgrades, asynchronous database-replica reads, arbitrary S3-provider conformance, Kubernetes/ECS deployment behavior, or database failover durability. Legacy storage conversion must precede scale-out. Interrupted imports are not automatically resumed elsewhere. The server processes were independently started and stopped on one machine; container image distribution and orchestration remain separate deployment work.
