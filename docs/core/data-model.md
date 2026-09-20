---
title: Model your data
description: Choose repository boundaries and a file schema that fit your application's access, lifecycle, and workflow.
---

Start with the thing your application needs to preserve. Then ask who should have access to it, what should change together, and when it should be retired. Those answers are more useful than starting with one repository per agent by default.

## Choose a repository boundary

Keep data together when it shares readers, writers, history, and retention. Split it when those requirements differ.

| Your application needs | A useful starting point | Tradeoff |
| --- | --- | --- |
| Independent jobs or agent sessions | One repository per run or session. | Easy credential isolation and cleanup; cross-run discovery lives in your database. |
| A project with several related outputs | One repository per project. | Convenient shared history; writers need coordination. |
| Persistent personal memory | One repository per user within the appropriate account. | Sessions share context; all repository readers can see the user's stored history. |
| Parallel work from the same baseline | A parent repository and a snapshot fork for each worker. | Independent credentials and writes; combining results is application work. |

For a research application, `acme/research/run-42` is a reasonable repository address. For a long-lived workspace, `acme/projects/launch-plan` may be a better fit. Artifacts does not require either naming convention.

:::caution[Directories are not access controls]
A credential for a repository can read its accessible Git history, not just `outputs/`. Put private inputs and public deliverables in separate repositories if they require different readers. A branch is not an isolation boundary either.
:::

Namespaces group repositories within an account. Use short, predictable names such as `research`, `projects`, or `memory`. Namespace and repository names are 1–100 bytes, start with a letter or digit, and otherwise contain letters, digits, `.`, `_`, or `-`. Lowercase ASCII names are a convenient portable convention.

## Map the application record to the repository

Keep a durable mapping in your application's database. For example, a `research_runs` record could contain:

```json title="Application-owned record (illustrative schema)"
{
  "id": "run_42",
  "tenant_id": "acme",
  "status": "running",
  "artifacts_account": "acme",
  "artifacts_namespace": "research",
  "artifacts_repo_name": "run-42",
  "artifacts_repo_id": "<id returned by create>",
  "published_commit": null
}
```

The application ID and repository ID serve different purposes. Use your stable application ID for your workflow; retain the server's repository ID to identify the actual resource. Store names as well because REST and Git routes address repositories by account, namespace, and name.

When the worker finishes, record the published commit SHA alongside the completed state. Readers can then fetch that exact version even if `main` advances. Avoid treating the latest branch head as an immutable result.

Do not store credentials in this record as ordinary data, or commit them into the repository. Issue short-lived repository credentials when a worker needs access and pass them through your secret-handling mechanism.

## Design files for both people and programs

A small, explicit file layout gives consumers a contract:

```text title="Example: a research run"
AGENTS.md                 worker instructions
run.json                  schema version, IDs, input provenance
inputs/
  brief.md                task brief
  sources.json            structured source list
outputs/
  report.md               human-readable result
  findings.json           machine-readable result
```

Use JSON for records another program will parse, Markdown for prose, and native formats for binary outputs. Add a `schema_version` from the beginning. Validate the manifest in your application before publication and after reading untrusted worker output.

```json title="run.json (your schema, not an Artifacts API resource)"
{
  "schema_version": 1,
  "run_id": "run_42",
  "project_id": "launch-plan",
  "input_revision": "brief-3",
  "outputs": [
    { "path": "outputs/report.md", "media_type": "text/markdown" },
    { "path": "outputs/findings.json", "media_type": "application/json" }
  ]
}
```

Store original output bytes when using Git. Encoding a PDF as base64 in a JSON file makes it harder for standard tools to use. The REST commit API accepts string content and is best suited to small text snapshots; it has no binary upload encoding field.

## Decide what changes together

One commit should contain a coherent version that a consumer can use: for example, the report, its machine-readable findings, and a manifest that references both. Publishing them together prevents a reader pinned to that commit from seeing mismatched versions.

**REST commits preserve untouched files.** Send the report you changed without resending `inputs/brief.md`. Use `deletes` for removal and `replace: true` only for deliberate full-tree replacement. Use Git for binary files, file-mode changes, larger updates, and merges. Read the [write contract](/artifacts/core/writing-files/) before building updates.

Supply `expected_head` when writing changes derived from a version you read. Competing writers then receive a conflict instead of silently replacing newer work. Use `Idempotency-Key` for retries of the same publication. Artifacts detects conflicts; your application or Git workflow still decides how to reconcile them.

## Plan schema evolution and discovery

When changing a manifest schema, update readers to accept the new version before writers start publishing it. Keep old-version readers or a migration path for historical commits. A file rename is also a contract change for consumers that know a path.

Use your application database or a derived index for queries such as “all completed reports for project X.” Artifacts lists repositories and reads files; it does not query JSON fields across repositories. Record the repository and commit behind each indexed result so you can rebuild the index and retrieve the source.

## Plan retention before collecting data

Deleting a file from the latest commit does not remove it from Git history. Deleting a repository blocks access and revokes its credentials, but retains immutable data needed by descendant snapshot forks. Artifacts does not provide a physical erasure API or automatic retention scheduler.

Choose boundaries that allow whole repositories to be retired when their work ends. For personal memory, define the difference between retracting a record from normal recall and removing its historical bytes. The [memory example](/artifacts/examples/personal-memory/) illustrates an explicit retraction model.

## A model you can implement

Before integrating, you should be able to name the repository's owner, credential audience, writer, canonical manifest, output paths, and retirement rule. You should also know which commit a consumer should read. With those choices made, move on to [integrating your application](/artifacts/core/integration/).
