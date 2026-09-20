---
title: Examples
description: See how the same repository primitives support short-lived work, long-lived memory, and independent handoffs.
---

These examples layer application conventions on top of Artifacts. An agent session, a memory, or a run manifest is your data model; Artifacts stores its files and publication history.

If you have not used the API yet, complete the [quickstart](/artifacts/getting-started/). For the design decisions behind these examples, read [integration guide](/artifacts/core/integration/).

## Choose a pattern

| Example | Repository boundary | What you will learn |
| --- | --- | --- |
| [Agent sessions](/artifacts/examples/agent-sessions/) | One repository per run. | Seed with REST, delegate Git access, and read the worker's result through REST. Includes the executable Go harness. |
| [Personal memory with MCP](/artifacts/examples/personal-memory/) | A user's long-lived memory repository. | Model related records and native output files, with a disposable local search index. Includes the runnable MCP server. |
| [Snapshot handoffs](/artifacts/examples/snapshot-handoffs/) | A baseline and independent child repositories. | Give several workers a captured starting point with separate credentials and lifecycles. |

## Adapt the example to your product

Keep the repository boundary aligned with access and retention. Replace example file schemas with ones your application validates. Store the repository mapping and completed commit SHA in your own database. Explicitly assign responsibility for worker completion, credential renewal/revocation, and cleanup.

The [API reference](/artifacts/core/api-reference/) documents the request and response contract. The [Developer tools](/artifacts/developer-tools/local-development/) section covers the local browser and verification workflow used to inspect these examples.
