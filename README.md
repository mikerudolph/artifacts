# artifacts

Git-compatible versioned file storage for agents. Each agent, and each session inside an agent, gets its own isolated repo. Standard `git` talks to it over HTTP. Metadata lives in Postgres; objects live on a local disk or any S3-compatible bucket.

You need: Go 1.25+, Docker, `git`, and `jq` (optional, used below to pull fields out of JSON).

## 1. Start it

```bash
git clone https://github.com/mikerudolph/artifacts.git
cd artifacts

docker compose up -d postgres

export DATABASE_URL=postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable
export ARTIFACTS_AUTH=none
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_PUBLIC_URL=http://localhost:8080
export ARTIFACTS_DEFAULT_ACCOUNT=local

go run ./cmd/artifacts migrate
go run ./cmd/artifacts serve
```

The server listens on `:8080`. Leave this terminal running.

`ARTIFACTS_AUTH=none` is for a single-user laptop. For anything shared, see [Auth](#auth).

`ARTIFACTS_PUBLIC_URL` is written into every repo `remote`. It must be a URL the agent process can reach (not `localhost` if the agent runs in another container).

## 2. Give an agent a session repo

Treat **one repo = one unit of work**. If you have 10 agents, or 10 sessions of one agent, create 10 repos. Do not share a repo across sessions.

```bash
export ARTIFACTS=http://localhost:8080/client/v4/accounts/local/artifacts
export NS=agents
export AGENT=researcher
export SESSION=sess-042
export REPO="${AGENT}-${SESSION}-work"
```

Create the repo (the namespace is created on first use):

```bash
CREATE=$(curl -sS -X POST "$ARTIFACTS/namespaces/$NS/repos" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"$REPO\",\"description\":\"$AGENT session $SESSION\"}")

echo "$CREATE" | jq .

export REMOTE=$(echo "$CREATE" | jq -r .result.remote)
export TOKEN=$(echo "$CREATE" | jq -r .result.token)
```

`REMOTE` looks like `http://localhost:8080/git/agents/researcher-sess-042-work.git`.
`TOKEN` looks like `art_v1_<hex>?expires=<unix>`. It is a **write** token, good for 24 hours. Treat it as a secret.

Hand `$REMOTE` and `$TOKEN` to the agent. That is the whole storage layer for this session.

## 3. The agent uses git

Git protocol v2 is not implemented. Every git command needs `-c protocol.version=1`.

```bash
# first commit
mkdir /tmp/$REPO && cd /tmp/$REPO
git init -b main
printf '# session work\n' > README.md
git add README.md
git commit -m "start session"

git -c protocol.version=1 \
    -c http.extraHeader="Authorization: Bearer $TOKEN" \
    push "$REMOTE" HEAD:main
```

Later in the same session:

```bash
git -c protocol.version=1 \
    -c http.extraHeader="Authorization: Bearer $TOKEN" \
    push "$REMOTE" HEAD:main
```

A new process that only needs to read:

```bash
# mint a short-lived read token (1 hour)
READ=$(curl -sS -X POST "$ARTIFACTS/namespaces/$NS/tokens" \
  -H 'Content-Type: application/json' \
  -d "{\"repo\":\"$REPO\",\"scope\":\"read\",\"ttl\":3600}")
export READ_TOKEN=$(echo "$READ" | jq -r .result.plaintext)

git -c protocol.version=1 \
    -c http.extraHeader="Authorization: Bearer $READ_TOKEN" \
    clone "$REMOTE" /tmp/${REPO}-ro
```

Basic auth also works if you cannot set headers (password = token secret, username ignored):

```bash
SECRET="${TOKEN%%\?expires=*}"
git -c protocol.version=1 clone "http://x:${SECRET}@localhost:8080/git/$NS/$REPO.git"
```

Read one file without cloning:

```bash
curl -sS "$ARTIFACTS/namespaces/$NS/repos/$REPO/file?ref=main&path=README.md"
```

## 4. How to partition work

| Thing | Use for |
|---|---|
| **Account** | Tenant. Local default is `local` (`ARTIFACTS_DEFAULT_ACCOUNT`). |
| **Namespace** | Environment or team: `agents`, `staging`, `prod`. |
| **Repo** | One agent session, one user task, or one fork. Name it `${agent}-${sessionId}-work`. |

Fork a reviewed baseline instead of copying files by hand:

```bash
# once: import or push a template into $NS/starter
curl -sS -X POST "$ARTIFACTS/namespaces/$NS/repos/starter/fork" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"$REPO\",\"description\":\"session $SESSION\",\"default_branch_only\":true}"
```

The fork response includes a new `remote` and `token`. The new repo has its own tokens; the baseline’s tokens do not work on it.

When the session ends, delete the repo:

```bash
curl -sS -X DELETE "$ARTIFACTS/namespaces/$NS/repos/$REPO"
```

That marks it deleting. Object cleanup is a separate job (`jobs.Runner.Delete`); for laptop use, leaving old session repos is fine until you prune.

## 5. Auth

Two different credentials:

| Who | Header | What it opens |
|---|---|---|
| Your harness / control plane | `Authorization: Bearer $ARTIFACTS_API_TOKEN` | Create repos, mint tokens, list, delete |
| The agent / `git` | `Authorization: Bearer $TOKEN` (the `art_v1_…` value) | Clone, fetch, push that **one** repo |

Locally, `ARTIFACTS_AUTH=none` skips the control-plane bearer. Git still needs a repo token.

On a shared box:

```bash
export ARTIFACTS_AUTH=token
export ARTIFACTS_API_TOKEN=$(openssl rand -hex 32)
go run ./cmd/artifacts token create   # prints the same value
```

Send that API token only to the orchestrator. Agents get repo tokens with `scope=read` or `scope=write` and a short `ttl` (seconds, min 60, max 1 year).

## 6. Routes the harness will call

Base: `/client/v4/accounts/<account>/artifacts`

| Method | Path | Purpose |
|---|---|---|
| POST | `/namespaces` | Create a namespace (`{"namespace":"agents"}`) |
| GET | `/namespaces` | List namespaces |
| POST | `/namespaces/:ns/repos` | Create a session repo; returns `remote` + write `token` |
| GET | `/namespaces/:ns/repos/:name` | Recover `remote` later |
| DELETE | `/namespaces/:ns/repos/:name` | 202 — mark session repo gone |
| POST | `/namespaces/:ns/repos/:name/fork` | Fork a baseline |
| POST | `/namespaces/:ns/repos/:name/import` | Import a public HTTPS (or `file://`) git remote |
| POST | `/namespaces/:ns/tokens` | Mint `{"repo","scope","ttl"}` |
| GET | `/namespaces/:ns/repos/:name/tokens` | List tokens |
| DELETE | `/namespaces/:ns/tokens/:id` | Revoke |
| GET | `/namespaces/:ns/repos/:name/file?ref=&path=` | Read a file |
| GET | `/namespaces/:ns/repos/:name/log?ref=` | Commit history |
| GET | `/namespaces/:ns/repos/:name/raw/:ref/:path` | File with sniffed Content-Type |

Git remotes: `{PUBLIC_URL}/git/{ns}/{repo}.git`

Repo names: start with a letter or digit; then letters, digits, `.`, `_`, `-`. Max 100 characters.

## 7. Run on S3 (any compatible API)

Same binary. Point object storage at MinIO, AWS, R2, Garage, B2, etc.

```bash
docker compose up -d postgres minio

export ARTIFACTS_STORAGE=s3
export S3_BUCKET=artifacts
export S3_ENDPOINT=http://127.0.0.1:9000
export S3_REGION=us-east-1
export S3_USE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=artifacts
export AWS_SECRET_ACCESS_KEY=artifacts-secret
# create the bucket once in the MinIO console at http://127.0.0.1:9001
#   (user/pass: artifacts / artifacts-secret)

export DATABASE_URL=postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable
export ARTIFACTS_AUTH=none
export ARTIFACTS_PUBLIC_URL=http://localhost:8080

go run ./cmd/artifacts migrate
go run ./cmd/artifacts serve
```

On AWS, omit `S3_ENDPOINT` and use a real bucket + IAM or static keys.

## 8. If something fails

| Symptom | Fix |
|---|---|
| `fatal: Could not read from remote repository` | Add `-c protocol.version=1`. Confirm `ARTIFACTS_PUBLIC_URL` matches the host you are curling. |
| `Authentication failed` | Git needs the **repo** token, not `ARTIFACTS_API_TOKEN`. Use the full `art_v1_…?expires=…` string as Bearer, or the secret before `?expires=` as Basic password. |
| `401` on REST with `ARTIFACTS_AUTH=token` | Send `Authorization: Bearer $ARTIFACTS_API_TOKEN`. |
| Push works, clone from another machine fails | `remote` was minted with a `PUBLIC_URL` that machine cannot reach. Set it before `serve` and recreate the repo. |
| `invalid name` | Repo/namespace must start with `[A-Za-z0-9]`. Prefer `researcher-sess042-work` over names with spaces or slashes. |

## Develop

```bash
make verify   # fmt, vet, lint, test, coverage ≥ 85%, file-length
```
