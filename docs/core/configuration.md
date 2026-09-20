---
title: Configuration
description: Run the service with explicit authentication, durable storage, and a reachable Git origin.
---

Artifacts runs with Postgres metadata and either filesystem or S3-compatible object storage. Multiple serving instances use one shared Postgres writer endpoint, the same S3-compatible bucket and prefix, and independent local cache directories. The server also requires the Git executable. Start with the [quickstart](/artifacts/getting-started/) for local use and [authentication](/artifacts/core/authentication/) for token setup.

## Service commands

| Command | Purpose |
| --- | --- |
| `go run ./cmd/artifacts dev` | Local unauthenticated REST, Git, and web UI; loopback only. |
| `go run ./cmd/artifacts dev --addr 127.0.0.1:8081` | Local mode at a different loopback address. Overrides listen/public URL configuration. |
| `go run ./cmd/artifacts serve` | Authenticated REST and Git. The development web UI is not mounted. |
| `go run ./cmd/artifacts bootstrap --account acme` | Ensure an account and injected control token after migration. |
| `go run ./cmd/artifacts token create --account acme` | Mint a hashed, account-bound control-plane token. |
| `go run ./cmd/artifacts migrate` | Run database migrations explicitly. Startup also runs them unless explicitly disabled. |
| `go run ./cmd/artifacts compact --account acme --namespace research --repo run-42` | Write a checkpoint pack for the repository. |

For a built executable, use `go build -o ./bin/artifacts ./cmd/artifacts` and replace `go run ./cmd/artifacts` with `./bin/artifacts`.

## Database, HTTP, and authentication

| Environment variable | Default / behavior |
| --- | --- |
| `DATABASE_URL` | Postgres DSN. Falls back to `ARTIFACTS_DATABASE_URL`. Configure it explicitly. |
| `ARTIFACTS_SKIP_MIGRATIONS` | `false`. Set `true` to skip automatic migrations; serving requires a current schema. |
| `ARTIFACTS_SHUTDOWN_TIMEOUT` | `30s`. Positive deadline for draining active requests on SIGTERM. |
| `ARTIFACTS_BOOTSTRAP_TOKEN` | Secret consumed only by `bootstrap`; never printed. |
| `ARTIFACTS_BOOTSTRAP_TOKEN_FILE` | Alternative mounted secret file for `bootstrap`; do not set both sources. |
| `ARTIFACTS_HTTP_ADDR` | `:8080` in `serve` mode. Set `127.0.0.1:8080` to keep the listener local. |
| `ARTIFACTS_PUBLIC_URL` | `http://localhost:8080`. Public origin used in returned Git remotes; set to your externally reachable HTTPS origin. |
| `ARTIFACTS_STREAM_IDLE_TIMEOUT` | `30s`. Positive duration measuring inactivity within Git/REST transfers. |
| `ARTIFACTS_AUTH` | `token`. `none` is restricted to `dev`; `serve` rejects it. |
| `ARTIFACTS_DEFAULT_ACCOUNT` | `local`. Default tenant for environment-token bootstrap. |
| `ARTIFACTS_API_TOKEN` | Optional bootstrap control token; startup stores its hash for the default account. Use the explicit bootstrap command for production provisioning. |

See [production deployment](/artifacts/core/deployment/) for image setup, database roles, health probes, and graceful shutdown.

`token create` can bootstrap an account without disabling authentication and runs migrations as needed. Supply its output to the client that needs REST access. The service validates against stored hashes; you do not need to set `ARTIFACTS_API_TOKEN` when using a CLI-created token.

For a remote deployment, terminate TLS at your ingress or reverse proxy and set `ARTIFACTS_PUBLIC_URL` to that HTTPS origin. Configure proxy body-size and timeout limits to accommodate the operations you use. Do not expose `dev` mode through a public proxy; it bypasses authentication and also enforces a local Host header.

## Filesystem object storage

```bash
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=/var/lib/artifacts/objects
export ARTIFACTS_CACHE_DIR=/var/cache/artifacts/repos
```

| Variable | Default | Durability |
| --- | --- | --- |
| `ARTIFACTS_STORAGE` | `fs` | Selects `fs` or `s3`. |
| `ARTIFACTS_DATA_DIR` | `./data` | Durable immutable objects for `fs`. Keep on persistent storage. |
| `ARTIFACTS_CACHE_DIR` | `./cache` | Disposable local bare repositories, reconstructable from durable state. |

Create and provision these example directories according to your service user's permissions. Do not confuse the object directory with the cache: removing the former can destroy repository history. Give cache storage enough space for reconstruction, staging, repacking, and active Git operations.

## S3-compatible object storage

```bash
export ARTIFACTS_STORAGE=s3
export S3_BUCKET=artifacts
export S3_REGION=us-east-1
export ARTIFACTS_CACHE_DIR=/var/cache/artifacts/repos
# Supply AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY through your secret manager.
```

| Variable | Behavior |
| --- | --- |
| `S3_BUCKET` | Required with `ARTIFACTS_STORAGE=s3`. |
| `S3_ENDPOINT` | Optional custom endpoint; falls back to `AWS_ENDPOINT_URL`. |
| `S3_REGION` | Falls back to `AWS_REGION`, then `us-east-1`. |
| `AWS_ACCESS_KEY_ID` | Access key; falls back to `S3_ACCESS_KEY`. |
| `AWS_SECRET_ACCESS_KEY` | Secret key; falls back to `S3_SECRET_KEY`. |
| `S3_PREFIX` | Optional object key prefix. |
| `S3_USE_PATH_STYLE` | Enables path-style addressing with `1`, `true`, `yes`, or `on`; useful for compatible local services. |

Provision the bucket and access policy before starting the service. Keep object storage private; clients use Artifacts REST or Git, not direct bucket access. Changing the backend, bucket, or prefix does not migrate existing object bytes.

## Multiple serving instances

Run the same release behind a load balancer with a common `ARTIFACTS_PUBLIC_URL`. REST and Git requests can reach either instance without session affinity. Configure all instances with the same Postgres writer endpoint, S3 bucket, prefix, and account/authentication configuration. Provision control-plane tokens with `token create` before starting the instances; token records and revocations are shared through Postgres.

Give each instance independent writable cache and temporary storage. Cache loss is recoverable, but reconstruction, Git operations, and compaction can require substantial local disk space. Do not use separate filesystem object directories as if they were shared durable storage. Route metadata reads and writes to the database writer, not asynchronously replicated read endpoints.

Competing writes may return conflicts even on different branches because publication checks a repository-wide sequence. REST returns HTTP 409; Git may reject a stale ref or receive HTTP 409 if publication loses the race. Fetch or read the current state and reconcile before submitting a new write. For an uncertain REST outcome, retry the original supported operation with its original idempotency key before deciding whether to create a new operation.

Background compaction uses a nonblocking Postgres advisory transaction lock per repository. Another instance skips overlapping compaction; ordinary publication does not take that maintenance lock. Compaction holds a database connection while it runs, so size connection pools and database capacity for all instances. Local concurrency and transfer limits apply per process rather than globally.

Convert legacy storage-version-1 repositories on a single instance before scaling out. Mixed-release rolling upgrades and zero-downtime upgrades are not established by this topology: stop old instances, run migrations with the new release, then start the new instances. Interrupted imports are not automatically resumed by another instance.

See [multi-instance verification](https://github.com/mikerudolph/artifacts/blob/main/reviews/multi-instance.md) for the exercised workload and its limits.

## Operate and recover

Back up **both Postgres and durable object storage**. They hold complementary parts of the publication contract: refs and metadata in Postgres, immutable packs in storage. A disposable cache alone is not a backup. Coordinate recovery so every ref in the restored database has its referenced pack data.

Compaction writes a checkpoint to speed reconstruction. It does not physically purge retained history or implement garbage collection. Snapshot descendants can still depend on a deleted parent's objects, so independent age-based object deletion can break them.

The current implementation does not provide mixed-release upgrade guarantees, metadata reads from database replicas, automatic recovery of interrupted imports, an automatic retention scheduler, or a physical erasure API. Read [storage architecture](/artifacts/storage/) before changing deployment topology or deleting stored data.
