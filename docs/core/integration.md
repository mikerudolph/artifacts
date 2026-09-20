---
title: Integrate into your app
description: Choose what belongs in a repository, connect it to an application record, and publish results your app can retrieve.
---

Start with one feature in your application: a generated report, a project workspace, or a set of files produced by a job. Artifacts stores those files and their versions. Your application keeps the record that explains who owns the work and what should happen next.

The integration is a small loop: create a repository, publish files, and save the commit SHA with the application record. Add a worker when you need one.

## Choose what belongs together

Use one repository for files that share readers, writers, and a lifecycle. A report job is a useful starting point: its brief and result stay together, and the repository can be retired when the job's history is no longer needed.

For a shared project, a repository per project may fit better. For persistent personal memory, a repository per user can span many sessions. These are application choices, not special Artifacts resource types.

Repository access includes readable history. A directory or branch cannot hide private inputs from a reader who can access the repository. Split files into separate repositories when their audiences differ.

## Connect the repository to your record

Keep a mapping in your application database. For a report job, it could look like this:

```json
{
  "id": "report-42",
  "status": "draft",
  "repository": {
    "account": "acme",
    "namespace": "research",
    "name": "report-42",
    "id": "<returned repository ID>"
  },
  "result_commit": null
}
```

The repository ID identifies the resource; its names form REST and Git addresses. When a result is ready, save its commit SHA in `result_commit`. Later edits can advance `main` without changing which version your record refers to.

Keep credentials in your secret-handling system, separate from ordinary application records.

## Create once, publish as the work changes

Your backend uses an account control token to create a repository. It can then publish and read files using REST. This is enough for an application that generates its own outputs.

The requests below are relative to `/client/v4/accounts/acme/artifacts`. Send the control token as a Bearer header. The [quickstart](/artifacts/getting-started/) provides executable commands; the [API reference](/artifacts/core/api-reference/) documents response fields.

```http
POST /namespaces/research/repos
Content-Type: application/json
Idempotency-Key: create-report-42

{"name":"report-42","issue_credential":false}
```

Persist the returned repository ID and remote. Then publish the initial brief:

```http
POST /namespaces/research/repos/report-42/commits
Content-Type: application/json
Idempotency-Key: seed-report-42

{"expected_head":"","files":[{"path":"brief.md","content":"Research the launch plan."}]}
```

The empty `expected_head` requires an unborn branch. Save the returned `sha`. When your app generates a report, publish `report.md` with that SHA as the next write's `expected_head`; the brief remains in place.

Choose an idempotency key for each logical create or commit operation and retain it until the outcome is known. If a response is lost, retry the same request with the same key and body. A new key describes a new operation. If a version check returns a conflict, read the current state and decide how to reconcile the change before submitting it again.

## Read the version your app selected

Use the returned commit SHA to retrieve a result:

```http
GET /namespaces/research/repos/report-42/file?ref=<commit-sha>&path=report.md
```

A successful file response contains the file's bytes. Parse or stream those bytes according to your file format. When consuming multiple files, use the same SHA for every read so the result stays consistent.

Your application decides when to record that SHA as the completed or approved result. Validate the required files and their contents before changing business state. Use your own database or derived index for queries such as “all completed reports for this project.”

## Add a worker with access to this repository

If another process produces the report, your backend issues a repository write credential with a suitable expiry and passes it to the worker through a secret channel. Keep the account control token in your backend.

The worker can use REST to publish text changes, or Git to clone, edit, and push. After publishing, it returns the commit SHA to your backend. Your backend reads that version, validates the result, and updates the application record.

A repository credential covers the repository's permitted content operations; it does not authorize account management. Account control tokens do not authenticate Git. See [authentication](/artifacts/core/authentication/) for issuing, using, renewing, and revoking repository credentials.

For a complete worker workflow, use the [agent session example](/artifacts/examples/agent-sessions/). For independent work from a shared starting point, see [snapshot handoffs](/artifacts/examples/snapshot-handoffs/).

## Make recovery part of the lifecycle

Your application database and Artifacts do not share a transaction. Retain the repository mapping and distinguish work that is being created, running, or awaiting validation. If a worker publishes but its completion message is lost, your backend can inspect the repository and reconcile the result.

Start with direct completion messages if they fit your workflow. When you need to discover publications independently, use the [polling event feed](/artifacts/core/api-reference/#publication-events), persist its cursor, and tolerate duplicate processing.

Revoke worker credentials when their task ends. Delete a repository when your retention policy allows, and retry failed cleanup using the saved mapping. Deletion blocks access; it does not physically erase historical bytes. Plan repository boundaries with that limitation in mind.

The [errors and limits guide](/artifacts/core/errors-and-limits/) owns detailed recovery behavior. The [examples](/artifacts/examples/) show file schemas and complete applications you can adapt.
