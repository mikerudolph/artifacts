---
title: Developer tools
description: Inspect local repositories and verify an integration while you build.
---

The local web UI and verification harness help you see what your code published. Your application integrates through the core REST and Git interfaces; these tools are optional aids for development.

## Browse the work

The [web UI](/artifacts/developer-tools/web-ui/) is included in `artifacts dev`. Navigate accounts, namespaces, repositories, branches, and directories; preview text files; inspect commits and WAL publications. It reads through the same REST surface your application uses.

It is a local inspection tool. It does not provide repository creation, file editing, user management, or a production administration console, and it is not served by `artifacts serve`.

## Verify your integration

The [local development guide](/artifacts/developer-tools/local-development/) covers CLI workflows, the read-only doctor command, and a complete REST → Git → REST verification run with redacted evidence and cleanup.

For service configuration and durable storage, use the [Core configuration reference](/artifacts/core/configuration/). For executable applications to adapt, go to [Examples](/artifacts/examples/).
