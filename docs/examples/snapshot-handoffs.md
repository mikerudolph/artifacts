---
title: Snapshot handoffs
description: Give independent workers the same captured baseline without sharing a writable repository.
---

Suppose two workers should explore different approaches to a launch plan. They need the same brief and source material, but each should have its own credentials and write history. A snapshot fork is a good fit.

## Publish a baseline

Use [manage repositories](/artifacts/core/repositories/) to create `research/baseline`, then [publish](/artifacts/core/writing-files/) the complete starter file set: `AGENTS.md`, `inputs/brief.md`, and `inputs/sources.json`.

Capture the baseline commit SHA in your application. If every child must start from exactly the same state, pause baseline writers or set `read_only: true` after seeding. Each fork captures the current published state at its own creation time; the fork endpoint does not accept a requested historical SHA.

## Fork for independent work

For the first worker, with `API` and `CONTROL_TOKEN` from [authentication](/artifacts/core/authentication/):

```bash
FORKED=$(curl --fail-with-body -sS -X POST \
  "$API/namespaces/research/repos/baseline/fork" \
  -H "Authorization: Bearer $CONTROL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"approach-a","default_branch_only":true}')
export REMOTE=$(printf '%s' "$FORKED" | jq -er '.result.remote')
export REPO_TOKEN=$(printf '%s' "$FORKED" | jq -er '.result.token')
unset FORKED
```

Repeat with a fresh destination name, `approach-b`, for the second worker, storing each remote and credential separately. The children remain in the baseline's account and namespace. Their credentials are scoped to their respective repositories.

Each worker uses [Git](/artifacts/core/git/) to add a report and push. A later change to the baseline does not move either child's refs. Their original files can be reconstructed from the captured parent lineage without copying all stored pack bytes at fork time.

## Compare published results

Have each worker return its pushed commit SHA. Read `outputs/report.md` from each child at the corresponding SHA and compare them in your application. Record which result you accepted and the exact repository/commit behind it.

There is no built-in review, pull request, or REST merge-back API. You can publish the accepted files into a separate result repository, or use Git to merge changes when appropriate. If publishing with REST, include the full desired result tree.

## Retire the work

Revoke the workers' credentials when they finish. Keep the child repositories while consumers need their results, then delete them according to your retention policy. Deleting the parent blocks access to the parent but retains objects needed to reconstruct descendants.

A fork is an access and lifecycle boundary, not a way to scrub sensitive content from inherited history. For a public deliverable that must contain only selected files, create a fresh repository and publish those files without inheriting the private history.
