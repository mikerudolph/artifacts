---
title: Agent sessions
description: Give a worker an isolated repository, then consume its Git-published output through REST.
---

Use a repository per agent session when each run should have independent credentials, history, and cleanup. The orchestrator keeps the task record in its database and stores the run's files in Artifacts.

## The handoff

1. The orchestrator creates `research/run-42` over REST and saves the returned repository ID and remote.
2. It publishes `AGENTS.md`, `inputs/brief.md`, and any other bootstrap files in one REST snapshot.
3. It issues a short-lived write credential for that repository and passes it to the worker through a secret channel.
4. The worker clones, produces `outputs/report.md`, commits, and pushes with Git.
5. It reports the pushed SHA. The orchestrator reads `outputs/report.md` over REST at that SHA and validates the result.
6. The orchestrator records completion and the SHA, revokes the credential, and later retires the repository under its retention policy.

```text
Orchestrator                 Artifacts                    Worker
    │  create + seed REST       │                           │
    ├──────────────────────────>│                           │
    │  remote + scoped credential ─────────────────────────>│
    │                           │<──────── clone / push ────┤
    │<────────────────────────────── completion + SHA ───────┤
    ├───── read file at SHA ────>│                           │
    │  validate + record result │                           │
```

The worker's ordinary Git edits preserve bootstrap files. If a worker uses REST instead, it needs trusted control-plane access or backend mediation and must submit the whole desired file tree. Repository credentials only work on Git.

## Run the executable reference

Start a local instance with the [quickstart](/artifacts/getting-started/), then from the project root:

```bash
export ARTIFACTS_URL=http://127.0.0.1:8080
export ARTIFACTS_ACCOUNT=local
go run ./examples/agent-harness doctor
go run ./examples/agent-harness
```

For an authenticated local `serve` instance, set `ARTIFACTS_API_TOKEN` to its control token in the client environment. The harness obtains its own repository credential. Use a disposable local target: the example creates, writes, and deletes a unique repository as part of its workflow.

To preserve sanitized verification evidence:

```bash
EVIDENCE_DIR=$(mktemp -d)
go run ./examples/agent-harness verify-core --evidence "$EVIDENCE_DIR"
```

Expect proof of REST creation, a REST commit and readback, real Git clone/push, REST readback of Git-authored content, refs/WAL visibility, and final repository absence after deletion. The evidence directory retains `report.json`, `report.md`, and `git.log` after client cleanup.

## What the reference teaches

The [Go harness source](https://github.com/mikerudolph/artifacts/tree/main/examples/agent-harness) separates the HTTP client, Git invocation, workflow, and redacted report. Use its transport and verification patterns as a reference while keeping your application's scheduling and task state in your own backend.

An expired credential should trigger a backend authorization check and replacement issuance. A lost completion message should trigger reconciliation of the expected output and published ref. A failed cleanup should retain the repository mapping for a later retry. These cases are covered in [integrate your application](/artifacts/core/integration/#handle-the-gaps-between-systems).

When the next session needs to continue from this work independently, use a [snapshot handoff](/artifacts/examples/snapshot-handoffs/).
