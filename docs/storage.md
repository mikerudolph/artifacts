# Storage architecture

Artifacts v2 is intentionally single-node. Postgres is the only publication authority. There is no gossip, rendezvous ownership, multi-node replication protocol, or object-store compare-and-swap.

## Write path

1. Git receive data is bounded and staged on disk in the repository cache.
2. Git validates and indexes the received objects.
3. The cache is repacked into one immutable `.pack` and `.idx` pair.
4. Both blobs are uploaded under tenant/repository content-addressed keys.
5. One Postgres transaction locks the repository, checks its status/read-only flag and expected WAL sequence, validates every expected old ref, inserts the WAL row and ref-update rows, and publishes all refs.
6. Only after that transaction commits is success returned to Git.

Object durability therefore precedes ref visibility. A failed multi-ref compare rolls back every ref and the WAL record. Uploaded but unpublished blobs are harmless garbage and can be collected later.

## Cache and checkpoints

Bare repositories under `ARTIFACTS_CACHE_DIR/{account}/{repo_id}` are disposable. A keyed process mutex and an OS file lock shared across processes serialize rebuild, Git RPC, receive, REST commit, and compaction for one repository. Each cache records the Postgres WAL sequence it represents and is checked with `git fsck`; missing, stale, or corrupt caches are evicted and reconstructed.

Rebuild order is:

1. parent snapshot lineage, limited to the captured parent sequence;
2. the newest usable checkpoint;
3. later immutable WAL packs;
4. current refs from Postgres and the repository's symbolic default-branch `HEAD`.

`artifacts compact --account A --namespace N --repo R` writes the current full pack as a checkpoint. Removing the cache directory does not remove durable repository data.

## Snapshot forks and deletion

A same-tenant snapshot fork records `(child, parent, parent_sequence)` and copies ref metadata in one transaction. It does not copy object bytes. Cache rebuild follows lineage only through the captured sequence, so later parent pushes are invisible to the child.

Deletion moves a repository through `deleting` to `deleted`, which blocks Git and REST lookup immediately and revokes its credentials. Immutable objects and metadata are retained so descendant forks remain reconstructable. Physical garbage collection must account for lineage reachability before deleting packs.

## Legacy data

Migration 001 remains unchanged. Migration 002 adds tenant ownership on repositories, storage version and WAL sequence, failure/deletion state, immutable pack/ref-update tables, checkpoints, fork lineage, and job lease fields. The legacy loose-object storer remains readable for migration/import compatibility; all normal v2 publication uses immutable packs.

HTTPS imports apply a hard per-file operating-system quota and stream object counting. The aggregate checkout-size cap is monitored during clone and checked again before publication; it is best-effort between monitor intervals, so deployments should retain an outer filesystem or container quota as defense in depth.
