# Artifacts goals

## Purpose

Make versioned files easy for teams to use in applications. An application should be able to create an isolated repository, publish changes, read an exact version, and delegate access without building its own versioning infrastructure.

Agents and sessions are important users of this model. The core interface should also be useful to ordinary application backends and Git clients.

## The application model

Keep the concepts an application must understand small:

1. A repository holds related files and their history.
2. A commit SHA identifies an exact version; a branch identifies a moving version.
3. A publication changes files and refs together. Clients can check the expected head and safely retry supported operations.
4. A repository credential delegates expiring read or write access through REST content routes and Git.
5. A snapshot fork starts independent work from captured history.
6. A publication feed lets consumers resume processing changes.

Pack files, cache reconstruction, checkpoints, and storage sequences belong to the implementation or diagnostic interfaces. Consumers of the event feed need its cursor, but ordinary file readers should not need to understand storage publication.

## What we optimize for

- **Easy integration.** Make the common workflow understandable from a small, complete example. Remove repeated coordination and recovery work from application teams.
- **Familiar Git behavior.** REST and Git share one history. Updating selected files preserves unrelated content; replacement and deletion are explicit.
- **Durable acknowledged writes.** Successful publication survives process restart and loss of local caches.
- **Safe delegation.** Workers receive access to their repository without receiving account-wide management credentials.
- **Predictable recovery.** Conflicts, invalid input, authorization failures, and uncertain outcomes give callers a clear next action.
- **Work proportional to the request.** Small edits should not republish the full repository. Small content reads should avoid downloading unrelated pack data where the supported read path permits it.
- **A maintainable service.** Prefer a small set of well-tested mechanisms with clear ownership over additional frameworks and operating modes.

## How we judge changes

| Promise | Evidence we expect |
|---|---|
| REST and Git share history | Write through either interface and read the same commit and files through the other. |
| File updates preserve unrelated work | Change one path and verify untouched files, modes, and history remain. |
| Writers can detect conflicts | Two writes using the same expected head cannot both advance that branch; intervening Git pushes are detected. |
| Supported retries are safe | Repeating a keyed create or commit after restart returns the original result without another publication. |
| Acknowledged data is durable | Remove disposable caches, restart, and reconstruct the published content. |
| Delegation stays within its boundary | Test account/repository separation, read versus write, expiry, revocation, and management-route restrictions. |
| Forks are stable snapshots | Advance and compact the parent; the child still reads its captured history. |
| Consumers can recover | Resume the event feed after restart and compaction without missing committed publications or splitting their ref changes. Consumers handle duplicates. |
| Small changes stay inexpensive | Measure uploaded bytes and storage reads on representative fixtures; explain cost changes with the implementation. |

These are acceptance criteria, not a claim that every possible workload or failure has been proved. Performance targets need an explicit workload and reproducible measurements before becoming release guarantees.

## Product boundaries

Artifacts stores versioned files. Applications own their business records, schemas, search indexes, worker scheduling, billing, and user authorization. A publication event does not mean a business task succeeded.

The current deployment is one serving node, Postgres, and durable filesystem or S3-compatible object storage. Multi-node serving requires its own design and verification; it is not an implied capability of disposable caches.

The web UI supports inspection and development. Developer interfaces remain the priority. SDK and local-setup improvements are deferred until explicitly brought back into scope. Webhook delivery is not part of the current polling event feed.

Do not broaden the product into a code-review platform, CI system, or application database without an explicit product decision.

## Maintaining this document

Change these goals when the intended product changes. Keep endpoint details and operational limits in the [documentation](docs/core/api-reference.md), and contributor instructions in [AGENTS.md](AGENTS.md).

Distinguish supported behavior, proposed work, and deferred work. Do not turn an aspiration or a passing small fixture into an unsupported product claim.
