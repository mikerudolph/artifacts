---
title: Get started
description: Run Artifacts locally and publish your first repository through REST and Git.
---

# Get started

Run a local Artifacts service, create an isolated repository, and read back a published file in a few minutes.

## Requirements

- Go 1.25 or newer
- Docker
- Git
- `jq` for the command-line examples (optional)

## Start the service

Start Postgres from the repository root:

```bash
docker compose up -d postgres
```

Configure local storage and run the development server:

```bash
export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache

go run ./cmd/artifacts dev
```

The development server listens on `http://127.0.0.1:8080` and serves the browser, REST API, and Git smart HTTP without authentication. It binds only to the loopback interface.

:::tip[Transfer timeouts]
Set `ARTIFACTS_STREAM_IDLE_TIMEOUT` to change the maximum inactivity allowed during a Git or REST transfer. The default is `30s`.
:::

## Create a repository

The local REST base is:

```bash
export API=http://127.0.0.1:8080/client/v4/accounts/local/artifacts
```

Create a repository for an agent session:

```bash
curl -sS -X POST "$API/namespaces/agents/repos" \
  -H 'Content-Type: application/json' \
  -d '{"name":"researcher-session-42","description":"session artifacts"}' | jq
```

The response includes a tenant-qualified Git remote and a short-lived write credential.

## Publish without Git

Publish a commit through the REST API:

```bash
curl -sS -X POST "$API/namespaces/agents/repos/researcher-session-42/commits" \
  -H 'Content-Type: application/json' \
  -d '{"message":"initial artifacts","files":[{"path":"README.md","content":"# Session 42\n"},{"path":"results/summary.txt","content":"ready\n"}]}' | jq
```

Read a file back at a named ref:

```bash
curl -sS "$API/namespaces/agents/repos/researcher-session-42/file?ref=main&path=README.md"
```

## Clone with Git

Set the `REMOTE` and `REPO_TOKEN` values from the create response, then clone with a Bearer credential:

```bash
git -c http.extraHeader="Authorization: Bearer $REPO_TOKEN" clone "$REMOTE"
```

Repository credentials are scoped to exactly one repository. Read credentials cannot push, and revoked or expired credentials are rejected.

## Next steps

- Follow the [agent onboarding flow](./onboarding/) to hand work between an orchestrator and agents.
- Read the [storage architecture](./storage/) to understand publication, reconstruction, snapshots, and deletion.
