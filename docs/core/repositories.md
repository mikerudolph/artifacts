---
title: Manage repositories
description: Create, find, configure, and retire the repositories your application owns.
---

All examples use `API` and `CONTROL_TOKEN` from [authentication](/artifacts/core/authentication/). In local `dev` mode the authorization header is ignored. A repository name is unique within its namespace and account.

## Create a repository

```bash
CREATED=$(curl --fail-with-body -sS -X POST "$API/namespaces/research/repos" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"run-42","description":"Launch research","default_branch":"main"}')

export REMOTE=$(printf '%s' "$CREATED" | jq -er '.result.remote')
export REPO_TOKEN=$(printf '%s' "$CREATED" | jq -er '.result.token')
printf '%s' "$CREATED" | jq '.result | del(.token, .credential.plaintext)'
unset CREATED
```

The response is HTTP `200`. `result` contains `id`, `name`, `description`, `default_branch`, `remote`, `token`, and `credential`. The credential object includes `id`, `plaintext`, `scope`, and `expires_at`; `token` is a compatibility plaintext alias. The credential is returned as plaintext only at issuance. Save the repository identity in your application and handle the token as a secret.

| Create field | Required | Behavior |
| --- | --- | --- |
| `name` | Yes | 1–100 bytes; starts with a letter or digit; remaining characters can also include `.`, `_`, and `-`. |
| `description` | No | Human-readable description. |
| `default_branch` | No | Defaults to `main`. |
| `read_only` | No | Defaults to `false`. See the bootstrap exception below. |
| `issue_credential` | No | Defaults to `true`; set `false` to create without a secret. |

Creation ensures the namespace exists, creates symbolic `HEAD`, and makes the empty repository ready. It does not create an initial commit. An immediate file or tree read has nothing to resolve until you [publish files](/artifacts/core/writing-files/).

## Organize namespaces

Create namespaces explicitly when you want them to exist before their repositories:

```bash
curl --fail-with-body -sS -X POST "$API/namespaces" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"research"}' | jq
```

The optional `jurisdiction` accepts `eu` or `us`. In the current implementation this is metadata; it does not select a storage region or enforce data residency. There is no namespace update or delete route.

## Find your repositories

Read one repository or search the names within a namespace:

```bash
export REPO_API="$API/namespaces/research/repos/run-42"
curl --fail-with-body -sS "$REPO_API" \
  -H "Authorization: Bearer $CONTROL_TOKEN" | jq '.result'

curl --fail-with-body -sS --get "$API/namespaces/research/repos" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  --data-urlencode 'search=run-' --data-urlencode 'sort=updated_at' \
  --data-urlencode 'direction=desc' --data-urlencode 'limit=50' | jq
```

Read responses include metadata such as `read_only`, `created_at`, `updated_at`, `last_push_at`, and `wal_sequence`. The internal repository lifecycle `status` is not serialized in this response. Do not build a client that expects a `result.status` field.

Repository and namespace lists use cursor pagination. Send `result_info.cursor` as the next request's `cursor` when non-empty. `limit` defaults to 50 and is capped at 200. Credential lists use a different pagination format; see the [API reference](/artifacts/core/api-reference/).

## Update settings

Only supplied settings change. For example, freeze an already seeded repository:

```bash
curl --fail-with-body -sS -X PATCH "$REPO_API/settings" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"description":"Completed launch research","read_only":true}' | jq '.result'
```

`description`, `default_branch`, and `read_only` are mutable. There is no repository rename route. Changing `default_branch` changes symbolic `HEAD`; it does not create a commit on a missing branch. Create or push that branch first.

:::note[Read-only bootstrap exception]
Git writes are blocked for a read-only repository. A control-token REST request permits an initial commit when its WAL sequence is zero, including on a newly created read-only repository. Repository credentials cannot write read-only repositories, including the initial commit. Later control-token REST writes are blocked. For an unambiguous freeze, publish the content first and then set `read_only: true`.
:::

## Retire a repository

```bash
curl --fail-with-body -sS -X DELETE "$REPO_API" \
  -H "Authorization: Bearer $CONTROL_TOKEN" | jq

curl -sS -o /dev/null -w '%{http_code}\n' "$REPO_API" \
  -H "Authorization: Bearer $CONTROL_TOKEN"
```

Delete returns `202` with `result.id` identifying the repository. The durable tombstone transaction runs before that response: normal REST and Git lookup are already blocked, and repository credentials are revoked. The follow-up lookup should return `404`.

The repository's `/jobs` route also becomes unreachable, so do not poll that route after deletion. Confirm absence through repository lookup instead. Repeated deletion of the same tombstoned repository is supported for cleanup retries.

Deletion retains immutable data and lineage metadata so existing snapshot forks remain reconstructable. There is no restore or physical purge API. See [storage architecture](/artifacts/storage/#snapshot-forks-and-deletion) before setting a retention policy.
