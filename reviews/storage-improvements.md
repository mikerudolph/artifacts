# Artifacts storage and publication improvements

Artifacts retains Postgres as its publication authority and its existing repository-scoped access model. The implementation improves publication size, content reads, and application event integration.

## Smaller publications and separate compaction

Git pushes and REST commits now create self-contained packs containing objects newly reachable from the updated refs, excluding objects reachable before the write. A ref-only write can publish an empty pack. Git still determines reachability and constructs the pack; the implementation does not directly reuse the incoming wire pack.

Full packs are built for imports, legacy conversion, and checkpoints. Checkpoints include the objects available in the synchronized cache, including inherited history. Checkpoint publication uses a freshly loaded repository sequence under the repository lock. Cache reconstruction and selective reads ignore checkpoints newer than a fork's recorded parent sequence.

The serving process discovers compaction work from Postgres at startup and once per minute. It handles up to eight repositories with at least 32 publications since their checkpoint. Failure leaves the work discoverable for a subsequent pass; server restarts do not lose an in-memory work queue. Manual compaction remains available. Old packs and publication records remain retained.

## Content reads fetch selected pack ranges

The application content reader snapshots refs and the publication sequence, validates and caches immutable pack indexes, and resolves Git objects using byte-range reads from filesystem or S3 storage. The reader checks decoded object hashes and keeps at most eight 256 KiB blocks for the current pack. Fork lineage and checkpoint selection use the same history resolver as full cache rebuilding.

Objects over 8 MiB use the existing disk-backed reader. Git clone, fetch, push, REST writes, and compaction still use a complete local Git cache. This is a selective content-read path, not a replacement for all Git serving operations. Indexes are cached on disk; range blocks live for the request.

## Applications can resume committed changes

`GET /namespaces/{namespace}/repos/{name}/events?after=0&limit=100` returns complete publications with their sequence, repository ID, timestamp, and ref transitions. It reads the same committed database records as publication; there is no separate best-effort notification write.

An event page never splits one publication's ref changes. Failed writes and successful idempotent retries create no extra events. Compaction creates no event. Consumers save `next_after` after processing and deduplicate with `(repo_id, sequence)` when replaying. Read/write repository credentials can consume their own feed. Events are retained with publication history; a deleted repository's feed is inaccessible. Webhook delivery is not included.

## Verification

The test suite covers cold selective reads, incremental pack size, delta decoding, large-object fallback, fork watermarks, corrupted index recovery, filesystem/S3 range contracts, transaction isolation of events, rollback, whole-publication pagination, idempotency, authentication boundaries, and durable compaction discovery.

The [live evidence](evidence/storage-improvements/README.md) contains 30 passing checks plus the core harness report. In its fixture, a 505,270-byte seed pack was followed by a 355-byte Git edit pack and a 337-byte REST edit pack. Index bytes and transport overhead are excluded. These values demonstrate that the unchanged large file was not republished; they are not a throughput benchmark.

The supported deployment remains a single serving node. Neither the Postgres authority nor credential setup is replaced, and SDK/local-setup work remains deferred.
