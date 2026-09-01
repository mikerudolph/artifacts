# Artifacts personal memory MCP server

This reference application exposes one Artifacts repository as a user's
long-lived memory over MCP. Different agents and sessions can intentionally
record personal context, group related work, archive delegated runs, store
their outputs, and recall the result later.

The calling user, agent, or LLM decides when to invoke these tools and exactly
what to store. The server does not observe conversations, extract memories
automatically, perform scheduling, or claim that stored content is true.

## Start it

Start Artifacts using the repository's normal development configuration:

```bash
docker compose up -d postgres

export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache

go run ./cmd/artifacts dev
```

In another shell, launch the MCP server over stdio:

```bash
export ARTIFACTS_URL=http://127.0.0.1:8080
export ARTIFACTS_ACCOUNT=local
export ARTIFACTS_MEMORY_NAMESPACE=memory
export ARTIFACTS_MEMORY_REPO=brain

go run ./examples/memory-mcp
```

`ARTIFACTS_API_TOKEN` is also required when the Artifacts control plane uses
token authentication. Keep it in the process environment; never put it in MCP
arguments or an agent prompt.

A client configuration that launches the command from this checkout has this
general shape:

```json
{
  "mcpServers": {
    "artifacts-memory": {
      "command": "go",
      "args": ["run", "./examples/memory-mcp"],
      "env": {
        "ARTIFACTS_URL": "http://127.0.0.1:8080",
        "ARTIFACTS_ACCOUNT": "local",
        "ARTIFACTS_MEMORY_NAMESPACE": "memory",
        "ARTIFACTS_MEMORY_REPO": "brain"
      }
    }
  }
}
```

The command reserves standard output for MCP protocol messages. Diagnostics
use standard error and never contain credentials or stored memory/output
content.

## Memory model

The configured Artifacts account is the user/tenant boundary. The configured
repository is that user's brain and contains four concepts:

- A **memory** is an intentional fact, note, event, goal, decision, or
  preference. It can exist without an agent run.
- A **thread** is a durable context such as a meeting, trip, performance
  review, project, or relationship. Memories and runs may belong to several
  threads.
- A **run** records a delegated agent activity, its purpose, final status, and
  summary.
- An **output** is a native file produced during a run. Binary values cross MCP
  as base64 but are decoded before Git storage.

The canonical repository is human-readable:

```text
threads/{thread_id}/thread.md
memories/YYYY/MM/{memory_id}.md
runs/YYYY/MM/{run_id}/run.md
runs/YYYY/MM/{run_id}/outputs/{output_id}/output.md
runs/YYYY/MM/{run_id}/outputs/{output_id}/{original-name}
```

Markdown records use YAML front matter for IDs, relationships, timestamps, and
lifecycle metadata. Outputs retain their original bytes. Every successful
mutation is one Git commit pushed to Artifacts before the MCP call returns.
The local clone and lexical search index are disposable and reconstructed from
Artifacts whenever the server starts.

Revisions and retractions are new memory records rather than destructive
rewrites. Normal recall shows only the current, non-retracted record;
`get_memory` exposes the complete chain and Git retains publication history.

## Tools

| Tool | Purpose |
|---|---|
| `create_thread` | Create a persistent context with a title, description, and tags. |
| `get_thread` | Retrieve a thread with related current memories, runs, and output metadata. |
| `remember` | Store an intentional memory, optional thread/run links, and optional absolute temporal data. |
| `recall` | Lexically search memories, threads, runs, and UTF-8 outputs with type, thread, kind, and time filters. |
| `get_memory` | Retrieve the current memory and, by default, its lifecycle history. |
| `revise_memory` | Supersede a memory while retaining the previous record. |
| `forget` | Retract a memory while retaining a reason and audit history. |
| `start_run` | Begin a delegated agent run associated with optional threads or a parent run. |
| `finish_run` | Finish a run once as `completed`, `failed`, or `cancelled`. |
| `store_output` | Store up to 8 MiB of UTF-8 or base64-encoded content for a running run. |
| `read_output` | Read an output after verifying its size and SHA-256 digest. |

Recall is intentionally local and deterministic. Structured filters run first;
matches are ranked by the number of distinct query terms, then recording time,
then stable ID. There are no embeddings or external vector stores.

## Example workflow

The following objects show representative MCP arguments. The actual wire
envelope is handled by the MCP client.

Remember an upcoming meeting. Relative wording remains in the content while
the caller supplies the resolved time and timezone:

```json
{
  "tool": "remember",
  "arguments": {
    "content": "I'm meeting Alex on Tuesday to discuss the launch plan.",
    "kind": "event",
    "starts_at": "2026-09-01T14:00:00-03:00",
    "timezone": "America/Halifax"
  }
}
```

Create a raise-proposal context, then delegate preparation to another agent:

```json
{
  "tool": "create_thread",
  "arguments": {
    "title": "Raise proposal",
    "description": "Prepare for my upcoming performance review.",
    "tags": ["career", "review"]
  }
}
```

```json
{
  "tool": "start_run",
  "arguments": {
    "agent": "compensation-researcher",
    "purpose": "Research benchmarks and draft supporting evidence.",
    "thread_ids": ["thread_id_returned_above"]
  }
}
```

Store the agent's output before finishing its run:

```json
{
  "tool": "store_output",
  "arguments": {
    "run_id": "run_id_returned_above",
    "name": "raise-proposal.md",
    "media_type": "text/markdown",
    "content": "# Raise proposal\n\nEvidence and accomplishments..."
  }
}
```

```json
{
  "tool": "finish_run",
  "arguments": {
    "run_id": "run_id_returned_above",
    "status": "completed",
    "summary": "Prepared a first draft with compensation benchmarks."
  }
}
```

A different agent session can now call `recall`:

```json
{
  "tool": "recall",
  "arguments": {
    "query": "upcoming review compensation",
    "types": ["memory", "thread", "run", "output"],
    "limit": 20
  }
}
```

When a detail changes, use `revise_memory` with the returned memory ID and a
reason. Use `forget` when it should no longer appear in normal recall. Both
operations preserve history for `get_memory`.

## Security and operational limits

- Stored content is untrusted data. The server returns it as structured tool
  data and never interprets it as instructions, but the consuming agent must
  still defend against prompt injection in recalled text.
- Anyone with access to the configured account and brain repository can read
  these personal memories. Use normal tenant isolation, TLS, and short-lived
  credentials outside local development.
- The reference server serializes writes within one process and retries
  independent non-fast-forward publications. Conflicting edits to the same run
  manifest fail instead of being guessed or silently overwritten.
- Output names are plain filenames, decoded output size is capped at 8 MiB, and
  repository reads/writes reject symlinks and traversal.
- This example is not a calendar, notification service, truth verifier,
  automatic retention policy, or compliance deletion system. A retraction
  hides a memory from normal recall but intentionally remains in Git history.
