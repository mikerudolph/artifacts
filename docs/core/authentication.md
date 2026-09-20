---
title: Authentication
description: Use account tokens for management and repository credentials for REST content and Git.
---

| Credential | Used by | Authorizes | Created with |
| --- | --- | --- | --- |
| Control-plane token | Your trusted backend or operator. | REST operations for one account. | `artifacts token create --account ACCOUNT` |
| Repository credential | A Git or REST worker. | Read, or read/write content, on one repository. | Repository creation or `POST /namespaces/{namespace}/credentials` |

Repository credentials work with REST file/tree/blob/raw/commit/history/refs reads and `POST /commits`. A read credential cannot publish; a write credential can. Management routes—including repository lookup/list/create/delete, settings, credentials, forks/imports, jobs, and WAL—still require an account control token. Account control tokens cannot replace repository credentials on Git remotes.

## Start an authenticated service

Use the same database and storage configuration as your local setup. Stop the local `dev` process before starting `serve` on the same port. From the repository root:

```bash
export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache
export ARTIFACTS_HTTP_ADDR=127.0.0.1:8080
export ARTIFACTS_PUBLIC_URL=http://127.0.0.1:8080

export CONTROL_TOKEN="$(go run ./cmd/artifacts token create --account local)"
export ARTIFACTS_AUTH=token
go run ./cmd/artifacts serve
```

The command stores a SHA-256 hash of the control token in Postgres. The plaintext is emitted once by the CLI; the assignment above captures it in the environment. Your client process needs that value through your normal secret mechanism. Never commit it or log it.

The token is bound to the account supplied to the command. The same value sent to another account's REST route is rejected. Multiple tokens can be created for an account. The current public CLI/API does not expose control-token expiry or revocation; account token lifecycle needs operator management. Repository credentials do have expiry and revocation.

`artifacts dev` bypasses both authentication planes and binds only to loopback. Use `serve` to test permission behavior. The development web UI is only mounted in `dev` mode.

## Authenticate REST requests

Send the control token in the Bearer header. The shell examples in the reference use these variables:

```bash
export API=http://127.0.0.1:8080/client/v4/accounts/local/artifacts

curl --fail-with-body -sS "$API/namespaces" \
  -H "Authorization: Bearer $CONTROL_TOKEN" | jq
```

With a remote deployment, use its HTTPS origin and the correct account. The `/client/v4` route is the API's path convention; this service runs on your infrastructure and does not require a Cloudflare account.

## Issue and revoke repository credentials

Issue a one-hour write credential for an existing repository:

```bash
CREATED_CREDENTIAL=$(curl --fail-with-body -sS \
  -X POST "$API/namespaces/research/credentials" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"repo":"run-42","scope":"write","ttl":3600}')

export REPO_TOKEN=$(printf '%s' "$CREATED_CREDENTIAL" | jq -er '.result.plaintext')
CREDENTIAL_ID=$(printf '%s' "$CREATED_CREDENTIAL" | jq -er '.result.id')
unset CREATED_CREDENTIAL
```

The issue response contains `id`, `plaintext`, `scope`, and `expires_at`. Creation, fork, and import return the same object in `result.credential`; `result.token` remains a compatibility alias for its plaintext. Repository creation accepts `issue_credential: false` to avoid issuing a credential. This is required when creating with an `Idempotency-Key`, so replay records never store secrets.

| Setting | Accepted values | Default |
| --- | --- | --- |
| `scope` | `read` for clone/fetch; `write` for clone/fetch/push. | `write` |
| `ttl` | Seconds, from `60` to `31536000`. | `86400` (24 hours), including when set to `0`. |

List metadata without retrieving secrets:

```bash
curl --fail-with-body -sS \
  "$API/namespaces/research/repos/run-42/credentials?state=all" \
  -H "Authorization: Bearer $CONTROL_TOKEN" | jq
```

Revoke a credential using its ID:

```bash
curl --fail-with-body -sS -X DELETE \
  "$API/namespaces/research/credentials/$CREDENTIAL_ID" \
  -H "Authorization: Bearer $CONTROL_TOKEN" | jq
unset REPO_TOKEN
```

Creating a replacement does not revoke previous credentials. Retain `result.credential.id` from creation for direct revocation, or create without a credential and issue only the scope/TTL your worker needs.

## Give Git a credential without changing the remote

Keep the remote URL returned by the API free of userinfo. With `REPO_TOKEN` in the environment, Git's `--config-env` option reads the header value from an environment variable:

```bash
export ARTIFACTS_GIT_AUTH="Authorization: Bearer $REPO_TOKEN"
git --config-env=http.extraHeader=ARTIFACTS_GIT_AUTH clone "$REMOTE"
unset ARTIFACTS_GIT_AUTH
```

Apply the same option to authenticated `fetch` and `push` commands. This avoids putting the secret itself in Git arguments or persisting it in `.git/config`. Use a Git version that supports `--config-env` (Git 2.31+), or a secure credential helper with HTTP Basic authentication; the repository credential is the Basic password and the username is ignored. Do not enable shell tracing around credentials.

## Understand enforcement

Each Git request and repository-authenticated REST content request checks the stored repository binding, tenant, scope, state, and expiry. An expired, revoked, wrong-repository, or cross-account credential is rejected with `401`. A read credential attempting a push or REST publication is rejected with `403`. Repository read-only state also blocks Git writes, even with a write credential.

The credential may include an `?expires=` suffix. Treat the whole value as secret; editing the suffix does not extend the stored expiry. Git credentials cover a repository's readable history and are not path- or branch-scoped. See [data model boundaries](/artifacts/core/data-model/#choose-a-repository-boundary) when different files need different audiences.
