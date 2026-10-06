# Verify binary commits

Use `verify-binary` for the multipart REST commit feature. Read the current contract in [writing files](../../../../../docs/core/writing-files.md) and use the same authentication, owned-target, evidence, and cleanup rules as the core drive.

Run doctor before mutation. The binary drive includes `verify-core`, then generates a disk-backed incompressible fixture and uploads it with text and an executable in one commit. `--size-mib` is the total raw content across those files, between 1 and 512. The default is 100. No entire binary body is buffered by the harness.

```sh
go run ./examples/agent-harness doctor
go run ./examples/agent-harness verify-binary --size-mib 100 --evidence reviews/evidence/binary-100
```

Require these assertions in the retained report:

- Multipart returns HTTP 201 with a SHA and sequence.
- SHA-pinned REST bytes match the generated file's byte count and SHA-256.
- Real Git fetch obtains the same bytes and executable mode.
- A real Git binary edit/push is visible through a SHA-pinned REST read.
- Repeating the original operation with a fresh multipart boundary after that push returns its original SHA and sequence.
- Changed input with the same key and stale expected head with a new key return 409.
- Retries and conflicts create no extra publications: the complete drive has four publications.
- Cleanup confirms that the uniquely named repository is inaccessible and client scratch is removed, with redacted evidence retained.

For the configured 512 MiB boundary, use `--size-mib 512` in a separate evidence directory. This repeats uploads for retry checks, creates Git caches, and retains historical packs in object storage until the owned storage itself is removed. Reserve several GiB of disk. Use disk-backed cache/scratch volumes; tmpfs usage is memory usage. A successful run proves the tested fixture and recovery path, not throughput or peak-memory bounds.

For an isolated built-image check with Postgres, MinIO, two serving instances, and disk-backed cache volumes, use:

```sh
docker build -t artifacts-binary-verify:local .
python3 scripts/verify-container.py --image artifacts-binary-verify:local --binary-size-mib 100 --binary-size-mib 512 --evidence reviews/evidence/binary-container-run
```

Choose a fresh evidence directory. This driver runs doctor and both binary drives, checks the Node and Python streaming examples, exercises cross-instance publication and cold-cache restart, and removes its owned containers, volumes, network, and client scratch in its cleanup path. Require `checks.json` to report success as well as both binary reports. Runtime source hashes and image identity are retained because an uncommitted working tree cannot be identified by its base commit alone.

Changes to resource handling also require targeted HTTP tests for malformed/missing/extra parts, known and unknown sizes, exact limits, stalled bodies, cancellation, admission exhaustion, and credential expiry/revocation during upload. Changes to publication require independent-cache same-key races, competing heads, cache reconstruction, and failures around the metadata transaction. The repository's automated tests cover these boundaries; do not replace the live drive with those tests.

For performance claims, separately record peak application and child-Git memory, scratch/cache disk, elapsed time, and object-store request/byte totals for a stated fixture. Keep client buffering out of those measurements. Capture process/container termination or restart evidence when claiming crash recovery; ordinary request completion does not prove it.

Unlinked staging files close on request completion or process exit. Lost responses still require replaying the whole operation with unchanged content and key. Cancellation during finalization does not prove rollback. No resumable upload sessions or orphan-pack garbage collection are provided.
