---
title: Quickstart
description: Start Artifacts locally, publish your first files over REST, and read a Git-authored result back through the API.
---

By the end of this guide, you will have a repository containing a brief and a report, with one shared history written through both REST and Git.

## Requirements

- Go 1.25 or newer
- Docker
- Git (2.31+ for the authenticated Git examples later in these docs)
- `curl` and `jq` for the commands below

Clone this project and run the server commands from its root:

```bash
git clone https://github.com/mikerudolph/artifacts.git
cd artifacts
```

## Start the service

In your first terminal, start Postgres and wait for it to be healthy:

```bash
docker compose up -d --wait postgres
```

Configure local storage and run the development server:

```bash
export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache

go run ./cmd/artifacts dev
```

Leave this process running. When it prints `listening on 127.0.0.1:8080`, the service is ready. Database migrations run at startup.

:::note[Local development mode]
`artifacts dev` serves REST, Git, and the optional web UI without authentication on loopback. This guide uses that local mode. For a deployed service, follow [authentication](/artifacts/core/authentication/) and use `serve`.
:::

## Create a repository

In a second terminal, set the API base and a unique repository name. The account is `local`, the namespace is `research`, and this repository represents one run.

```bash
export API=http://127.0.0.1:8080/client/v4/accounts/local/artifacts
export REPO="quickstart-$(date +%s)"
```

Create the repository and capture the returned remote. Creating a repository also creates the namespace when needed.

```bash
CREATED=$(curl --fail-with-body -sS -X POST "$API/namespaces/research/repos" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg name "$REPO" '{name: $name, description: "My first Artifacts repository"}')")

export REMOTE=$(printf '%s' "$CREATED" | jq -er '.result.remote')
printf '%s' "$CREATED" | jq '.result | {id, name, default_branch, remote}'
unset CREATED
export REPO_API="$API/namespaces/research/repos/$REPO"
```

Expect HTTP `200` and a result with your repository name, `default_branch: "main"`, and a remote ending in `/git/local/research/<your-repo>.git`. The response also contains an initial write credential in `result.token`; the filtered output above keeps it out of the terminal. Local `dev` mode does not need it.

The repository is still empty. Its first branch commit is created in the next step.

## Publish without Git

Publish a brief and a starter report together:

```bash
curl --fail-with-body -sS -X POST "$REPO_API/commits" \
  -H 'Content-Type: application/json' \
  -d '{"message":"Seed the research run","files":[{"path":"inputs/brief.md","content":"# Brief\nResearch the launch plan.\n"},{"path":"outputs/report.md","content":"# Report\nWork in progress.\n"}]}' | jq
```

Expect HTTP `201`, `success: true`, a 40-character `result.sha`, and `result.sequence: 1`. That SHA is a normal Git commit. The sequence records the repository's first durable publication.

:::note[REST writes preserve untouched files]
`POST /commits` updates only supplied paths. Remove files explicitly with `deletes`; use `expected_head` to detect concurrent changes and `Idempotency-Key` for safe retries. Full-tree replacement requires `replace: true`. See [write files](/artifacts/core/writing-files/).
:::

## Read the published files

List the output directory at `main`:

```bash
curl --fail-with-body -sS --get "$REPO_API/tree" \
  --data-urlencode 'ref=main' --data-urlencode 'path=outputs' | jq '.result'
```

The result includes the resolved `commit` and an entry named `report.md`. Read its bytes:

```bash
curl --fail-with-body -sS --get "$REPO_API/file" \
  --data-urlencode 'ref=main' --data-urlencode 'path=outputs/report.md'
```

```text title="Expected file content"
# Report
Work in progress.
```

File reads return bytes, not a JSON envelope. There is no need to download or unpack storage objects.

## Continue with Git

Clone into a scratch directory, change the report, and push. The brief stays in the repository because this is an incremental Git edit.

```bash
WORK_DIR=$(mktemp -d)
git clone "$REMOTE" "$WORK_DIR/repo"
printf '# Report\nThe launch plan is ready.\n' > "$WORK_DIR/repo/outputs/report.md"
git -C "$WORK_DIR/repo" add outputs/report.md
git -C "$WORK_DIR/repo" -c user.name='Quickstart' \
  -c user.email='quickstart@example.com' commit -m 'Complete the report'
git -C "$WORK_DIR/repo" push origin main

export PUBLISHED_SHA=$(git -C "$WORK_DIR/repo" rev-parse HEAD)
```

Read the exact Git commit through REST:

```bash
curl --fail-with-body -sS --get "$REPO_API/file" \
  --data-urlencode "ref=$PUBLISHED_SHA" --data-urlencode 'path=outputs/report.md'
```

You should see `The launch plan is ready.` This is the integration loop: your application can create and read work through REST while a worker uses standard Git tools. See [work with Git](/artifacts/core/git/) for authenticated commands.

## Clean up the tutorial repository

When you are finished with this repository, delete it:

```bash
curl --fail-with-body -sS -X DELETE "$REPO_API" | jq
curl -sS -o /dev/null -w '%{http_code}\n' "$REPO_API"
```

Delete returns HTTP `202`; the following lookup should print `404`. This blocks repository access and revokes its credentials. It does not physically purge retained history. Your scratch clone remains at `$WORK_DIR/repo`; remove that directory when you no longer need the local copy. Stop your `dev` process with Ctrl+C when you finish.

## If something fails

| Symptom | Check |
| --- | --- |
| Cannot connect to Postgres. | Run `docker compose ps postgres`; check it is healthy and port 5432 is available. |
| Cannot connect to port 8080. | Check the first terminal for a startup error. If the port is occupied, use `dev --addr 127.0.0.1:8081` and change `API` accordingly. |
| `jq` reports a parse error. | Inspect the HTTP response; confirm you are using a JSON endpoint. `/file` returns raw bytes. |
| File or tree returns `404`. | Confirm the initial commit succeeded and use the same repository and ref. |
| Repository creation returns `409`. | Pick a new `REPO` value and repeat creation. |

## Next steps

- [Core concepts](/artifacts/core/concepts/) explains accounts, repositories, commits, and refs.
- [Model your data](/artifacts/core/data-model/) helps you choose what belongs in one repository.
- [Integrate your application](/artifacts/core/integration/) connects the API to your backend and workers.
