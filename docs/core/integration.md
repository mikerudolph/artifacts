---
title: Integrate your application
description: Connect repository lifecycle, workers, and published results to your backend.
---

This guide assumes you have completed the [quickstart](/artifacts/getting-started/) and chosen a [repository boundary](/artifacts/core/data-model/). The basic integration has three participants: your backend owns the workflow, a worker produces files, and Artifacts stores the published history.

## Keep the control plane in your backend

Your backend uses an account-bound control token for REST. It creates repositories, issues repository credentials, and reads results on behalf of authorized application users. A worker that only needs Git gets a short-lived credential for its assigned repository.

Do not expose the account control token to a browser or an untrusted agent. It authorizes REST operations across that account. If a client needs a file, authorize that request in your backend and proxy the file response, or provide repository-level Git access when that is appropriate.

## Build a small HTTP adapter

There is no client SDK required. This server-side JavaScript helper works with a runtime that provides `fetch`. It treats API failures as failures even if a body is valid JSON and keeps credentials out of error text.

```js title="artifacts.mjs"
const origin = process.env.ARTIFACTS_URL ?? 'http://127.0.0.1:8080';
const account = process.env.ARTIFACTS_ACCOUNT ?? 'local';
const controlToken = process.env.ARTIFACTS_API_TOKEN;
const base = `${origin.replace(/\/$/, '')}/client/v4/accounts/${encodeURIComponent(account)}/artifacts`;

export async function artifacts(path, { method = 'GET', body } = {}) {
  const response = await fetch(`${base}${path}`, {
    method,
    headers: {
      ...(controlToken ? { Authorization: `Bearer ${controlToken}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(30_000),
  });
  const envelope = await response.json();
  if (!response.ok || !envelope.success) {
    const code = envelope.errors?.[0]?.code ?? 'unknown';
    throw new Error(`Artifacts request failed: HTTP ${response.status}, code ${code}`);
  }
  return envelope.result;
}
```

Use this helper for JSON endpoints. The successful `/file`, `/blob`, and `/raw` responses are bytes and need a separate reader. The helper's 30-second timeout is an application choice; imports or large operations may need a different deadline. It deliberately does not retry writes.

## Create, seed, and delegate

Create a repository once for your application record, then persist the returned ID and remote with that record. Use a unique name derived from a stable run ID. Do not log the create response: its `token` is a secret.

```js title="Create an isolated unit of work"
import { artifacts } from './artifacts.mjs';

const namespace = 'research';
const repoName = 'run-42'; // Use your own unique run ID.
const repos = `/namespaces/${namespace}/repos`;
const repo = await artifacts(repos, {
  method: 'POST',
  body: { name: repoName, description: 'Research run 42' },
});
const repoPath = `${repos}/${encodeURIComponent(repo.name)}`;

const bootstrap = await artifacts(`${repoPath}/commits`, {
  method: 'POST',
  body: {
    message: 'Seed research inputs',
    files: [
      { path: 'AGENTS.md', content: 'Write the result to outputs/report.md.\n' },
      { path: 'inputs/brief.md', content: '# Brief\nResearch the launch plan.\n' },
    ],
  },
});

const credential = await artifacts(`/namespaces/${namespace}/credentials`, {
  method: 'POST',
  body: { repo: repo.name, scope: 'write', ttl: 3600 },
});
// Persist repo.id, repo.name, repo.remote, and bootstrap.sha in your application.
// Pass repo.remote and credential.plaintext to the worker through a secret channel.
// Keep credential.id so the backend can revoke it after the handoff.
```

Creation also issues an initial write credential with a 24-hour TTL. You can use that credential or mint a shorter-lived one as above. If you mint a new one, the initial credential remains valid until expiry or revocation; see [credential lifecycle](/artifacts/core/authentication/#issue-and-revoke-repository-credentials).

## Publish and consume the result

The worker clones the repository, changes files, commits, and pushes. Have it report its pushed commit SHA to your backend only after push succeeds. Fetch the expected result at that SHA, validate the file format and your schema, and then mark the application record complete.

For a REST-only worker, publish a complete snapshot and use the returned `sha`. A repository credential cannot authenticate REST; a REST worker needs trusted backend mediation or account-level control access.

Pin related reads to the same SHA. Reading `main` once for a manifest and again for its output could span two publications. The tree endpoint returns its resolved `commit`, which you can reuse for subsequent file requests.

There are no webhooks or completion callbacks in the current API. Completion can come from your worker protocol, or your backend can poll refs with bounded backoff. A new commit alone does not mean the business task succeeded; validate your completion contract.

## Handle the gaps between systems

Your application database and Artifacts do not share a transaction. Make intermediate states explicit, for example `creating → seeded → running → validating → completed`, with a failure state and a reconciliation job.

| Failure window | Recovery approach |
| --- | --- |
| Create succeeds, but your process loses the response. | Look up the intended unique name and compare it with the application mapping. Issue a fresh credential if needed; never assume a `409` proves ownership. |
| Seed succeeds, but the request times out. | Read the branch and expected files before retrying. A repeated REST commit creates another commit. |
| Worker pushes, but its completion message is lost. | Reconcile the published ref and your result manifest, then validate the output. |
| Git push is rejected after another writer advances the branch. | Fetch and reconcile the changes. Retry only if the result is still valid; surface actual conflicts. |
| A credential expires while work is running. | Your backend issues a replacement for the same repository after checking the job is still authorized. |

For a timeout or a `500`, the outcome can be uncertain. An acknowledged response can be lost after publication. Inspect state before retrying, and avoid assuming that every server error is transient. See [errors and limits](/artifacts/core/errors-and-limits/).

## Close the lifecycle

Revoke worker credentials after a task ends. Keep the repository while consumers need its content. If another task should continue independently, create a [snapshot fork](/artifacts/core/forks-and-imports/) and give it its own credentials.

When your retention policy allows, delete the repository and confirm lookup returns `404`. Deletion hides the repository immediately; it is not a physical purge of historical data. Persist cleanup failures in your application so they can be retried without losing the resource mapping.

For a complete executable REST → Git → REST implementation, see the [agent session example](/artifacts/examples/agent-sessions/).
