# Artifacts

**Versioned files for your applications.**

Artifacts gives your application Git-compatible repositories through a REST API. Create a repository, update files, read any committed version, and give a worker access to just the repository it needs.

Use REST and ordinary Git clients against the same history—for generated documents, agent workspaces, or application-managed files.

[Documentation](https://mikerudolph.github.io/artifacts/) · [API reference](docs/core/api-reference.md) · [Examples](docs/examples/index.md)

## What you can do

- **Save changes through REST.** Update selected files without overwriting the rest of the repository.
- **Work with Git.** Clone, commit, and push with ordinary Git clients.
- **Read an exact version.** Use a commit SHA to retrieve files from that point in history.
- **Delegate work.** Issue expiring, repository-scoped read or write credentials.
- **Branch out.** Fork a repository at a captured version for independent work.
- **Follow changes.** Resume a publication feed from a saved cursor.

## Try it locally

You need Go 1.25+, Docker with Compose, Git, `curl`, and `jq`.

In your first terminal:

```bash
git clone https://github.com/mikerudolph/artifacts.git
cd artifacts
docker compose up -d --wait postgres

export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache

go run ./cmd/artifacts dev
```

Leave the server running. Migrations run automatically. Development mode serves REST, Git, and the browser at [127.0.0.1:8080](http://127.0.0.1:8080), without authentication and bound to loopback only.

## Create, write, and read

In a second terminal, create a repository for one research run:

```bash
API=http://127.0.0.1:8080/client/v4/accounts/local/artifacts
REPO="research-$(date +%s)"
REPO_API="$API/namespaces/demo/repos/$REPO"

REMOTE=$(curl --fail-with-body -sS -X POST "$API/namespaces/demo/repos" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg name "$REPO" '{name: $name, issue_credential: false}')" \
  | jq -er '.result.remote')
```

The namespace `demo` is created automatically. Publish a file:

```bash
curl --fail-with-body -sS -X POST "$REPO_API/commits" \
  -H 'Content-Type: application/json' \
  -d '{"message":"Start research","files":[{"path":"report.md","content":"Research started.\n"}]}' \
  | jq '.result.sha'
```

The response contains a normal Git commit SHA. Read the file back:

```bash
curl --fail-with-body -sS --get "$REPO_API/file" \
  --data-urlencode 'ref=main' --data-urlencode 'path=report.md'
```

```text
Research started.
```

## Continue with Git

In the same terminal, clone that repository, edit the file, and push:

```bash
WORK_DIR=$(mktemp -d)
git clone "$REMOTE" "$WORK_DIR/repo"
printf 'Research complete.\n' > "$WORK_DIR/repo/report.md"
git -C "$WORK_DIR/repo" add report.md
git -C "$WORK_DIR/repo" -c user.name='Demo' \
  -c user.email='demo@example.com' commit -m 'Complete research'
git -C "$WORK_DIR/repo" push origin main

SHA=$(git -C "$WORK_DIR/repo" rev-parse HEAD)
curl --fail-with-body -sS --get "$REPO_API/file" \
  --data-urlencode "ref=$SHA" --data-urlencode 'path=report.md'
```

```text
Research complete.
```

Your application wrote through REST, a worker updated the file through Git, and REST read the exact result. Both interfaces share one history.

See the [full quickstart](docs/getting-started.md) for directory browsing, cleanup, and troubleshooting.

## Connect your application

Run `artifacts serve` with [authentication configured](docs/core/authentication.md) for a deployed service. Control-plane credentials manage account resources; repository credentials give workers access to one repository's content and Git operations.

When publishing files, use `expected_head` to detect concurrent changes and `Idempotency-Key` to safely retry commits. Untouched files are preserved; deletions and full replacement are explicit. See [writing files](docs/core/writing-files.md) for the request formats.

Start with [application integration](docs/core/integration.md) and [data modeling](docs/core/data-model.md), or explore the [working examples](docs/examples/index.md).

## How it works

Artifacts runs as one serving node backed by Postgres and durable filesystem or S3-compatible object storage. Postgres records published history; object storage holds immutable Git data. Local Git caches can be rebuilt. Writes upload incremental packs, and background maintenance creates checkpoints.

See [storage architecture](docs/storage.mdx) and [configuration](docs/core/configuration.md) for deployment and operational details.

## Developing Artifacts

With Docker running, run the verification suite from the repository root:

```bash
make verify
```

This runs formatting, vet, lint, race tests, coverage checks, and Go file-size checks. Read [AGENTS.md](AGENTS.md) for contributor guidance and [GOALS.md](GOALS.md) for product direction.
