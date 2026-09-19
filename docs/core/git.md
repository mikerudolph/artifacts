---
title: Work with Git
description: Clone, edit, branch, and push with standard Git tools against the same history your backend reads over REST.
---

Artifacts exposes Git smart HTTP at the tenant-qualified remote returned by repository creation:

```text
{PUBLIC_URL}/git/{account}/{namespace}/{repo}.git
```

Use that returned remote instead of reconstructing it from storage paths or repository IDs. The service must have a reachable `ARTIFACTS_PUBLIC_URL` in `serve` mode.

## Clone and authenticate

Set `REMOTE` and `REPO_TOKEN` from the create response or an [issued repository credential](/artifacts/core/authentication/). With Git 2.31+:

```bash
export ARTIFACTS_GIT_AUTH="Authorization: Bearer $REPO_TOKEN"
git --config-env=http.extraHeader=ARTIFACTS_GIT_AUTH clone "$REMOTE" workspace
cd workspace
```

The header value stays in the environment rather than Git arguments or the saved remote. Include the `--config-env` option again on network operations; it does not persist configuration. If your tooling uses a credential helper and Basic auth, use the repository credential as the password.

In local `artifacts dev` mode, you can omit authentication entirely. An empty repository can be cloned, but it has no files until you commit or seed it over REST.

## Publish an incremental change

```bash
mkdir -p outputs
printf '# Report\nThe launch plan is ready.\n' > outputs/report.md
git add outputs/report.md
git -c user.name='Research worker' -c user.email='worker@example.com' \
  commit -m 'Publish the report'
git --config-env=http.extraHeader=ARTIFACTS_GIT_AUTH push origin HEAD:main
git rev-parse HEAD
```

This adds or updates the report while preserving unrelated files. The returned local SHA is a published result only after push succeeds. Send that SHA back to your backend so it can read the result through [REST at a pinned version](/artifacts/core/reading-files/#pin-a-version-for-related-reads).

Git also supports binary files, file modes, deletes, and larger file sets than the bounded REST snapshot endpoint. The receive stream is capped at 512 MiB; this is a transfer limit, not a recommended file size or an unlimited storage guarantee.

## Branch from an existing version

```bash
git switch -c revision-a
# Edit files, then stage and commit them.
git --config-env=http.extraHeader=ARTIFACTS_GIT_AUTH push -u origin revision-a
```

A branch shares the repository's credentials and history. Use branches for related lines of work with the same access audience. Use a [snapshot fork](/artifacts/core/forks-and-imports/) for independent repository credentials or a separate lifecycle.

## Coordinate concurrent writers

Before changing a shared branch, fetch its current state. If a push is rejected because another writer advanced it, fetch and reconcile using normal Git tools:

```bash
git --config-env=http.extraHeader=ARTIFACTS_GIT_AUTH fetch origin
git rebase origin/main
# Resolve actual conflicts and validate the resulting files before pushing.
git --config-env=http.extraHeader=ARTIFACTS_GIT_AUTH push origin HEAD:main
```

Do not force-push automatically to make a job succeed. Decide whether concurrent work can merge safely, or serialize the branch in your application. Independent agent runs often fit separate repositories better than a shared writable branch.

Artifacts atomically validates the expected old refs and publication sequence before exposing updated refs. A failed multi-ref comparison rolls back the publication. This protects the storage transaction; your application still needs to resolve conflicting intent.

## Finish the handoff

After work ends, your backend can revoke the credential by ID. In the worker, clear temporary secret variables:

```bash
unset ARTIFACTS_GIT_AUTH REPO_TOKEN
```

Revocation prevents subsequent Git access but does not remove a worker's existing clone. Treat local workspace cleanup as part of your worker lifecycle.

## Diagnose a failed Git operation

| Symptom | Likely next check |
| --- | --- |
| `401` | Credential missing, expired, revoked, or bound to another repository/account. |
| `403` on push | Read-scoped credential or a read-only repository. |
| Repository not found | Account/namespace/name in the remote, or repository deletion. |
| Non-fast-forward rejection | Fetch and reconcile another writer's commits. |
| Transfer terminates after inactivity | Client/network progress and `ARTIFACTS_STREAM_IDLE_TIMEOUT`. |

Avoid HTTP tracing that includes authorization headers. The [verification harness](/artifacts/developer-tools/local-development/#verify-the-public-workflow) captures sanitized Git evidence for the complete interoperability workflow.
