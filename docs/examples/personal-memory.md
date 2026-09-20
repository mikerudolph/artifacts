---
title: Personal memory with MCP
description: A runnable reference application that stores intentional memories, related work, and native outputs in one versioned repository.
---

The `examples/memory-mcp` application exposes an Artifacts repository through MCP. Several agent sessions can intentionally store and recall a user's context. The caller decides what to record; the server does not observe conversations or extract memories automatically.

This is a useful contrast to a repository per session. Here, continuity belongs to the user and spans sessions, so the repository is long-lived. The configured account is the tenant boundary; all readers with repository access can read its stored history.

## Start the server

Start Artifacts using the [quickstart](/artifacts/getting-started/). Then run the MCP server from the project root in another terminal:

```bash
export ARTIFACTS_URL=http://127.0.0.1:8080
export ARTIFACTS_ACCOUNT=local
export ARTIFACTS_MEMORY_NAMESPACE=memory
export ARTIFACTS_MEMORY_REPO=brain
go run ./examples/memory-mcp
```

It uses stdio for the MCP protocol, so configure an MCP client to launch that command with this working directory and environment. It is not an interactive shell prompt or a web server. For authenticated Artifacts, also supply `ARTIFACTS_API_TOKEN` through the client process environment; do not put credentials in an agent prompt.

The [example README](https://github.com/mikerudolph/artifacts/blob/main/examples/memory-mcp/README.md) contains a client configuration and the complete tool contract.

## Understand the application schema

| Record | Meaning | Relationships |
| --- | --- | --- |
| Memory | Intentional fact, note, event, goal, decision, or preference. | Can link to threads and a run; can exist without a run. |
| Thread | Durable context such as a project, meeting, or relationship. | Groups memories and runs; records can belong to several threads. |
| Run | A delegated activity with purpose, status, and summary. | Can link to threads or a parent run. |
| Output | A native file produced during a run. | Belongs to a run, with metadata and a digest. |

These types are files defined by the example, not additional Artifacts API resources:

```text
threads/{thread_id}/thread.md
memories/YYYY/MM/{memory_id}.md
runs/YYYY/MM/{run_id}/run.md
runs/YYYY/MM/{run_id}/outputs/{output_id}/output.md
runs/YYYY/MM/{run_id}/outputs/{output_id}/{original-name}
```

Markdown records use YAML front matter for schema fields, relationships, timestamps, and lifecycle metadata. Outputs retain their native bytes. Every successful mutation is one Git commit pushed before the tool call returns.

## Walk through a memory workflow

Use the MCP client's tools in this order. Returned IDs connect the calls; representative arguments below omit the MCP wire envelope.

| Tool | Representative arguments | Result to keep |
| --- | --- | --- |
| `create_thread` | `{"title":"Launch plan","tags":["launch"]}` | Thread ID. |
| `remember` | `{"content":"The launch needs a technical review.","kind":"note","thread_ids":["<thread-id>"]}` | Memory ID. |
| `start_run` | `{"agent":"researcher","purpose":"Prepare the review brief","thread_ids":["<thread-id>"]}` | Run ID. |
| `store_output` | `{"run_id":"<run-id>","name":"brief.md","content":"# Review brief\n..."}` | Output ID. |
| `finish_run` | `{"run_id":"<run-id>","status":"completed","summary":"Prepared the review brief"}` | Completed run record. |
| `recall` | `{"query":"launch technical review","types":["memory","thread","run","output"],"limit":20}` | Related records and output metadata. |

Use `read_output` to retrieve an output after its size and SHA-256 digest are verified. `get_thread` collects related current records; `get_memory` can return the revision/retraction chain.

## Separate durable files from derived search

The local clone and lexical search index are disposable and rebuilt from Artifacts at startup. Recall applies structured filters, then ranks by matching query terms, recording time, and stable ID. There are no embeddings or external vector stores in this example.

This is the general integration pattern: keep canonical records and native files in a repository, and build a query layer appropriate to your product. A different application could use a database or search service while still retaining the source commit behind indexed results.

## Revisions, concurrency, and limits

`revise_memory` writes a superseding record; `forget` records a retraction. Normal recall hides superseded or retracted memories, while history remains readable. Retraction is not physical erasure.

The server serializes writes within its process and retries independent non-fast-forward publications. Conflicting edits to the same run manifest fail. Run completion is final (`completed`, `failed`, or `cancelled`); store outputs before finishing the run.

`store_output` supports up to 8 MiB of decoded UTF-8 or base64 content. Binary input is decoded before Git storage. This is an **example-specific MCP limit**, separate from the core REST commit's 1 MiB bound. Output names and repository paths are validated against traversal and symlinks.

Stored content is untrusted input for consuming agents. This example provides persistence and lexical recall, not truth verification, scheduling, notifications, or a compliance deletion system. Use [model your data](/artifacts/core/data-model/) to adapt its access and retention choices.
