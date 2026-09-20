# Artifacts

Artifacts gives every agent and session an isolated, Git-compatible artifact repository. Postgres is the publication authority, object storage holds immutable Git packs, and local bare repositories are disposable caches.

[Documentation](https://mikerudolph.github.io/artifacts/) · [Quickstart](docs/getting-started.md) · [Data modeling](docs/core/data-model.md) · [Examples](docs/examples/index.md)

Requirements: Go 1.25+, Docker, Git, and optionally `jq`.

## Five-minute local start

```bash
docker compose up -d postgres

export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache
# Optional: maximum inactivity within a Git or REST transfer (default 30s).
export ARTIFACTS_STREAM_IDLE_TIMEOUT=30s

go run ./cmd/artifacts dev
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). `artifacts dev` serves the browser, REST API, and Git smart HTTP without authentication and only binds to a loopback address. The browser uses the REST API to navigate branches and directories, preview bounded text files, and download binary or large files. Repository materialization from WAL packs stays on the server.

Create a repository and publish initial files without installing Git:

```bash
export API=http://127.0.0.1:8080/client/v4/accounts/local/artifacts

curl -sS -X POST "$API/namespaces/agents/repos" \
  -H 'Content-Type: application/json' \
  -d '{"name":"researcher-session-42","description":"session artifacts"}' | jq

curl -sS -X POST "$API/namespaces/agents/repos/researcher-session-42/commits" \
  -H 'Content-Type: application/json' \
  -d '{"message":"initial artifacts","files":[{"path":"README.md","content":"# Session 42\n"},{"path":"results/summary.txt","content":"ready\n"}]}' | jq

curl -sS "$API/namespaces/agents/repos/researcher-session-42/file?ref=main&path=README.md"

# Resolve a branch and list a directory without handling Git object IDs.
curl -sS "$API/namespaces/agents/repos/researcher-session-42/tree?ref=main&path=results" | jq
```

REST commits update selected files and preserve untouched paths. Use `deletes` for removal, `expected_head` for concurrency checks, and `Idempotency-Key` for safe publication retries. For full-tree replacement, explicitly set `replace: true`.

The create response contains a tenant-qualified remote and a short-lived REST-content/Git write credential (including its ID and expiry in `credential`):

```text
http://127.0.0.1:8080/git/local/agents/researcher-session-42.git
art_v1_<secret>?expires=<unix>
```

In normal `serve` mode Git requires that repository credential. It is bound to exactly one repository; the stored tenant, repository, scope, state, and expiry are checked for every operation.

```bash
git -c http.extraHeader="Authorization: Bearer $REPO_TOKEN" clone "$REMOTE"
```

## Production-style authentication

Create a tenant-bound, hashed control-plane token:

```bash
export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_AUTH=none
CONTROL_TOKEN=$(go run ./cmd/artifacts token create --account local)

export ARTIFACTS_AUTH=token
go run ./cmd/artifacts serve
```

Send `Authorization: Bearer $CONTROL_TOKEN` to REST. Control-plane tokens are stored only as SHA-256 hashes. Repository credentials use the `/credentials` routes (the prior `/tokens` names remain aliases).

## Operations

```bash
go run ./cmd/artifacts migrate
go run ./cmd/artifacts compact --account local --namespace agents --repo researcher-session-42
make verify
```

Writes upload incremental packs. The serving process checks once a minute for repositories with 32 publications since their checkpoint and compacts up to eight per pass. The command above also creates a checkpoint on demand. Old packs are retained for forks and publication history. REST content reads use cached indexes and remote byte ranges, with a full-cache fallback for objects over 8 MiB. Applications can resume committed changes through the [publication event feed](docs/core/api-reference.md#publication-events). Cache contents under `ARTIFACTS_CACHE_DIR` can be deleted at any time and are reconstructed from snapshot lineage, checkpoints, WAL packs, and Postgres refs.

Read [the example harness](examples/agent-harness/main.go) for REST → Git → REST readback.
