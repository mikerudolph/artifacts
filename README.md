# Artifacts

Artifacts gives every agent and session an isolated, Git-compatible artifact repository. Postgres is the publication authority, object storage holds immutable Git packs, and local bare repositories are disposable caches.

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

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). `artifacts dev` serves the browser, REST API, and Git smart HTTP without authentication and only binds to a loopback address.

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
```

The create response contains a tenant-qualified remote and a short-lived write credential:

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

Compaction creates a checkpoint pack. Cache contents under `ARTIFACTS_CACHE_DIR` can be deleted at any time and are reconstructed from snapshot lineage, checkpoints, WAL packs, and Postgres refs.

Read [onboarding](docs/onboarding.md) for the agent/session workflow, [storage architecture](docs/storage.md) for durability details, and [the example harness](examples/agent-harness/main.go) for REST → Git → REST readback.
