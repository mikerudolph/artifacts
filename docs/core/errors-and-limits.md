---
title: Errors & limits
description: Validate requests early, distinguish permission failures, and reconcile uncertain writes before retrying.
---

## Read errors deliberately

JSON endpoints wrap failures in `success: false`, `result: null`, and an `errors` array. Successful file/blob/raw endpoints return bytes. Check HTTP status before deciding how to decode a response, and do not assume every non-JSON response came from Artifacts; a proxy can fail first.

| HTTP status | API code | Meaning and next action |
| --- | --- | --- |
| `400` | `10101` | Invalid name or JSON body, including unknown fields, malformed JSON, and excessive JSON body size. Check the payload. |
| `400` | `10103` | Invalid credential TTL. Use seconds in the supported range. |
| `400` | `10100` | Invalid scope, state, sort, direction, jurisdiction, or other mapped input. |
| `400` | `10104` | Invalid or disallowed import URL. Use a public HTTPS remote without userinfo. |
| `400` | `10106` | Upstream import requires authentication; authenticated imports are unsupported. |
| `401` | `10100` | REST authentication rejected. Check the control token and route account. |
| `404` | `10200` | Resource/ref/file missing or repository hidden by its lifecycle state. The generic message is `File not found`. |
| `409` | `10201` | Name already exists. Reconcile ownership before reusing a resource. |
| `409` | `10302` | Repository is busy with an import/fork operation. Inspect state before retrying. |
| `500` | `10400` | Generic server error, including some validation and publication failures. Inspect request and state; do not blindly retry. |
| `502` | `10401` | Import upstream unavailable, timed out, or exceeded limits. |

Git uses its own protocol responses: invalid/expired/revoked credentials return `401`; insufficient write scope or read-only state returns `403`. Do not expect a REST envelope from Git endpoints.

:::caution[Some validation failures currently return 500]
Empty/oversized file sets, duplicate or invalid paths, and excessive decoded commit content can map to the generic internal error. Publication conflicts and read-only REST writes can also surface as generic failures. The current contract is not a universal `400` for validation or `409` for every write conflict. Validate in your client and inspect server diagnostics.
:::

## Request and transfer limits

| Surface | Limit / default | Integration consequence |
| --- | --- | --- |
| REST commit files | 1–100 files. | Use Git for a larger tree; an empty snapshot is unsupported. |
| REST commit content | 1 MiB total decoded string bytes. | Count UTF-8 bytes, not characters. |
| JSON request body | 2 MiB. | Includes JSON escapes, paths, and metadata; unknown fields are rejected. |
| Git receive stream | 512 MiB. | Split unusually large publications or reconsider artifact size. |
| Stream inactivity | 30 seconds by default. | Configurable with `ARTIFACTS_STREAM_IDLE_TIMEOUT`; not a fixed total transfer duration. |
| Import operation | 2 minutes, 1,000,000 objects, 512 MiB size budget. | Set outer quotas and suitable client/proxy deadlines. |
| Namespace/repository name | 1–100 bytes. | Start with a letter/digit; remaining letters/digits or `.`, `_`, `-`. |
| Repository credential TTL | 60–31536000 seconds. | Omitted or `0` defaults to 86400 seconds. |
| Namespace/repository page | Default 50, maximum 200. | Follow cursor pagination. |
| Credential page | Default 30, maximum 100. | Follow `page`/`per_page` pagination. |
| History listing | Default 20 entries, offset 0. | Choose a reasonable client page size. |
| Web UI text preview | 256 KiB. | A developer-tool preview bound; API file reads can stream larger files. |

Import size enforcement includes a hard per-file operating-system quota and monitored aggregate size, with a final check before publication. Aggregate enforcement is best-effort between monitor intervals. See [configuration](/artifacts/core/configuration/) and [storage](/artifacts/storage/) for deployment boundaries.

## Retry according to the operation

Read requests can usually be retried with bounded exponential backoff after transient network failures. Keep a pinned SHA for multi-file reads so retries do not silently select a new branch version.

Writes have no general idempotency-key support. A timeout may happen after a commit or repository was created. Resolve the intended repository and ref, verify expected content, and retry only after deciding what actually happened. Treat a repeated name's `409` as a prompt to reconcile, not automatic success.

REST snapshot writes have no expected-head field. Two competing snapshots need application coordination even if each request succeeds. Git clients can fetch and merge or rebase after a rejected push. Never turn an automated retry into an unconditional force push.

## Diagnose by layer

For connection failures, check the service process, address, database, and proxy first. For `401`, confirm the token plane and account. For `404`, confirm repository creation and the first commit before debugging the file path. For a cold read, allow for cache reconstruction.

Use `/refs` to inspect the published head and `/wal` to inspect publication sequence only when storage diagnostics are relevant. Capture status, API error code, repository identity, and commit SHA in application logs. Redact control tokens, repository credentials, authorization headers, and credential-bearing URLs.

The [local verification harness](/artifacts/developer-tools/local-development/#verify-the-public-workflow) distinguishes reachability, authentication, response-shape, product, assertion, cleanup, and evidence failures.
