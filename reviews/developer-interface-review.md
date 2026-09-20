**Artifacts developer interface review**

Reviewed 2026-09-19 against commit `c3db1f6`. Scope: REST, Git, authentication, local setup, and application integration; the development UI is supporting tooling. This is a source and documentation review, not a running-service assessment or a usability study. Priorities and effort estimates are engineering judgments, not measured customer demand. Links point to the reviewed implementation and documentation.

Artifacts already has a useful foundation: applications and Git workers share the same versioned files, a commit identifies a coherent result, and repositories provide an isolation boundary. The largest simplification opportunity is to make that foundation usable without each team building its own coordination, credential, and recovery layer.

The intended application model should be small: **choose a repository, read a version, publish changes against that version, and delegate access to that repository.** Git remains a first-class way to do the same work. Storage publication sequences, pack objects, and cache reconstruction should be optional diagnostic knowledge.

**Priority order**

| Priority | Simplification | Application work removed | Relative effort |
| --- | --- | --- | --- |
| P0 | Atomic file changes with an expected head | Full-tree reconstruction and application-only writer serialization | Large |
| P0 | Repository credentials for REST content operations | A custom backend proxy for every delegated REST worker | Medium–large |
| P0 | Idempotent create and publication | Bespoke recovery after lost responses | Large |
| P1 | Consistent validation and conflict errors | Guessing whether a failure needs correction, reconciliation, or retry | Small–medium |
| P1 | Explicit credential issuance with complete metadata | Finding the initial credential by listing, and managing unwanted credentials | Small–medium |
| P1 | A supported client and machine-readable contract | Repeated transport, pagination, encoding, and response parsing code | Medium |
| P1 | Stable operation lookup for imports and failures | Ambiguous polling and inaccessible failure details | Medium–large |
| P1 | A reproducible authenticated local profile | Separate hand-built development and permission-test setups | Medium |
| P2 | Consistent names, lists, and resource handles | Endpoint-specific exceptions | Small–medium, plus compatibility work |
| P2 | Pin reads and forks through a common version interface | Repeated SHA plumbing and handoff ambiguity | Medium; pinned forks need storage design |
| P2 | Bounded binary publication over REST | Installing and orchestrating Git for a small binary output | Medium–large |

P0 means foundational for a broadly useful application interface, not that existing Git integrations must stop. P1 removes recurring integration friction. P2 follows once the central contract is dependable. Effort includes authorization, compatibility, and failure handling; these are not calendar estimates.

**1. Make updating files safe and ordinary.**

Today `POST /commits` accepts a complete tree, with 1–100 string-content files and a 1 MiB decoded-content limit. Updating only `outputs/report.md` also removes an existing `inputs/brief.md` from the new version. It cannot publish an empty tree. Writing to a missing branch creates a root commit rather than branching from an existing version. These are legitimate snapshot semantics, but surprising defaults for application updates. See [the write contract](../docs/core/writing-files.md), [commit input types](../internal/types/storage.go), and [commit construction](../internal/repository/commit.go).

Add an atomic change operation with explicit writes and deletes, preserving all untouched paths. Require an `expected_head` for updates; use an explicit “branch must not exist” condition for initial publication. A stale head should produce a conflict containing the current head, without publishing any part of the request. Branch creation from an existing version should take an explicit base commit.

The internal publication path already compares refs and sequences. That protects publication integrity, but it does not prove a client's content was derived from the latest head: a stale snapshot submitted after another writer finishes can still replace that writer's files. Carry the client precondition through the durable publication boundary. See [publication types](../internal/types/storage.go) and [REST commit publication](../internal/repository/commit.go).

Keep full-tree replacement available under an explicit `replaceSnapshot` operation in the client, preserving the existing endpoint's behavior for compatibility. Expose two clearly different intents—change selected paths and replace all files—rather than silently changing what existing requests mean. An SDK alone cannot safely emulate server-side atomic changes.

Acceptance: two writers starting from the same head cannot both succeed; updating one report preserves unrelated inputs; deleting the last file is expressible; a new branch can retain its intended parent history. Preconditions must also detect intervening Git pushes.

**2. Let repository access work across REST and Git.**

A repository credential currently authorizes Git only. REST content reads and writes require an account control token, so a small REST worker needs either broad account access or a custom proxy in the application's backend. The integration guide explicitly requires this mediation. See [REST middleware](../internal/api/auth.go), [credential enforcement](../internal/auth/repository.go), and [application integration](../docs/core/integration.md).

Accept repository credentials on a deliberately bounded set of REST content routes. Start with file/tree/history reads and commit publication: `read` permits content reads; `write` adds publication. Keep repository creation, credential issuance, deletion, imports, and other management operations under control-plane authorization. Reuse the existing stored repository binding, expiry, state, and read-only checks.

This simplifies the worker contract to a repository address and an expiring credential, independent of transport. Do not reinterpret all existing management routes as repository-write permissions. Decide and document the route permission matrix before implementation. Applications still authorize their users; whole-repository credentials still expose readable history, and are not suitable substitutes for path-specific sharing.

Acceptance: the same delegated credential can read via REST and clone via Git; read access cannot publish; revocation takes effect on both transports; cross-repository and cross-account use fails.

**3. Give retries a server-defined contract.**

Create, seed, and publication can succeed before their responses are lost. Current guidance tells teams to inspect names, refs, and files before retrying; repeated commits can create additional history. Both example clients also implement their own resource and response handling. See [failure windows](../docs/core/integration.md), [the harness client](../examples/agent-harness/client.go), and [the memory client](../examples/memory-mcp/artifacts_client.go).

Support caller-supplied idempotency keys for repository creation and publication first, then fork/import. Bind keys to tenant, operation, target, and a request digest. Repeating the same request returns the original outcome; reusing a key for different input returns a specific conflict. Publish the retention window and behavior after expiration. Durable outcome tracking must survive a process restart and concurrent duplicate requests.

Idempotency and expected-head checks solve different problems: the former prevents repeating one intended action; the latter prevents overwriting an intervening action. An idempotent replay of an already successful publication should return that original success even if the branch has since advanced.

Offer optional initial files during repository creation after this contract is established. This removes the common create→seed failure window. Prefer exposing the repository as ready only when the seed is durable, or return an explicit operation handle. Do not advertise an atomic bootstrap if it is just two ordinary client calls. Credential issuance should remain explicit, which also avoids requiring creation retries to replay secret plaintext.

Acceptance: dropping a successful response and retrying creates one resource or publication, including across restarts. This reduces Artifacts-specific reconciliation; it does not create a transaction with the application's own database.

**4. Make errors actionable before adding more endpoints.**

Several commit validation failures use untyped errors and therefore become generic `500` responses. `ErrCASConflict` also lacks an explicit mapping in the domain error mapper. Malformed create/fork/import bodies are mapped to invalid-name errors, and all mapped missing resources say “File not found.” These are direct sources of unnecessary retry and debugging code. See [commit validation](../internal/repository/commit.go), [domain mapping](../internal/api/envelope/domain.go), [create handling](../internal/api/repos.go), and [fork/import handling](../internal/api/fork.go).

Define stable error categories for invalid input, payload limit, unauthorized, forbidden, missing resource, head conflict, duplicate resource, and internal failure. Return field/path details for correctable input and current-head details for conflicts. Reserve `500` for server failures. Preserve numeric codes for existing consumers; introduce readable identifiers additively if needed. An API error should not require interpreting prose to choose the next action.

Do not add a blanket “retryable” flag that encourages unsafe write retries. Describe recovery per operation and connect transport retries to the idempotency contract. Document that streaming failures after response headers cannot be replaced with a JSON error envelope.

Acceptance: malformed JSON, duplicate file paths, oversized content, stale heads, and missing repositories produce distinct, documented responses that clients can handle without matching messages.

**5. Remove implicit and inconsistent credential work.**

Create/fork/import return a `token` string, while explicit issuance returns `id`, `plaintext`, `scope`, and `expires_at`. Ordinary repository creation always mints a 24-hour write credential, including for REST-only callers. The integration example then mints a shorter-lived credential, leaving the initial one valid. Identifying the initial credential for revocation requires listing. See [creation](../internal/service/repo.go), [result types](../internal/types/repo.go), [issuance](../internal/service/token.go), and [credential lifecycle documentation](../docs/core/authentication.md).

Use one credential result shape everywhere. Add an explicit credential option to creation, with selectable scope/TTL or no credential. In a new contract, make no credential the default; preserve legacy issuance until callers migrate. Return the ID whenever a secret is issued. Keep secrets out of ordinary repository objects.

Also add control-token list/revoke and optional expiry to the CLI. The existing public workflow offers creation without the corresponding lifecycle operations, leaving teams to manage rotation out of band. Keep this operator-facing until there is a concrete need for a broader management API.

Acceptance: a REST-only integration creates no unused Git secret; every issued credential can be revoked by its returned ID; teams can rotate account tokens using supported commands.

**6. Ship a small supported client around a checked contract.**

The integration guide starts by asking teams to write a `fetch` adapter, and the two Go examples independently implement envelopes, authentication, URL building, and error handling. This is useful evidence of repeated work within this repository, though not a measure of external adoption. See [the integration adapter](../docs/core/integration.md), [harness transport](../examples/agent-harness/client.go), and [memory transport](../examples/memory-mcp/artifacts_client.go).

Provide an OpenAPI description checked against the mounted routes, plus one supported client in the first adopting team's language. TypeScript is a reasonable candidate given the existing integration guide; confirm actual team demand before maintaining several SDKs. The client should configure origin/account/namespace once, return repository handles, expose typed errors, iterate lists, distinguish byte streams from JSON, and accept cancellation/deadlines. Retry writes only with the supported idempotency contract.

Keep the handwritten layer small and task-oriented: `create`, `files.read`, `files.list`, `commitChanges`, `replaceSnapshot`, `credentials.create`, and `fork`. Retain access to raw HTTP and Git. Avoid a parallel domain vocabulary that renames repositories to workspaces and commits to artifacts without removing any concepts.

Acceptance: a team can implement create→delegate→publish→read at a SHA without copying transport code from the examples. A generated client alone is insufficient if it preserves every low-level exception as application work.

**7. Make long-running outcomes discoverable independently of repositories.**

Imports and forks execute synchronously while also recording durable jobs. Job lookup is nested beneath a reachable repository; failed/deleted resources can make that route unavailable. Repository `status` is omitted from JSON. Deletion returns `202` and a repository ID, although tombstoning and credential revocation are completed before the response, and the ID is not an operation handle. See [routes](../internal/api/server.go), [job handling](../internal/jobs/jobs.go), [deletion](../internal/jobs/delete.go), [repository serialization](../internal/types/repo.go), and [the lifecycle reference](../docs/core/api-reference.md).

First expose tenant-authorized operation lookup by operation ID that remains available when the resource is failed or deleted, with bounded retention. Return explicit repository readiness where clients need it. Give potentially slow imports an asynchronous option backed by durable execution; merely returning `202` before continuing in a request goroutine would not solve recovery. Fast snapshot forks may remain synchronous.

For a future contract, return a completed deletion result or `204` for the completed access-removal action. Use `202` only when there is remaining work and a lookup handle. Keep logical deletion and physical erasure explicitly distinct.

Acceptance: a disconnected importer can reconnect and determine the outcome; a failed import remains diagnosable; deletion responses have an unambiguous meaning.

**8. Collapse setup choices and test the real permission model locally.**

The quickstart needs Docker/Postgres, a separate Go server, environment setup, and shell requests. `dev` intentionally disables both authentication planes, while the UI exists only in that mode. A successful tutorial therefore does not exercise the production credential split. See [quickstart](../docs/getting-started.md), [configuration](../docs/core/configuration.md), and [Compose services](../docker-compose.yml).

Provide a single documented local startup entry point that starts dependencies, waits for readiness, applies migrations, and prints the API origin and next command. Add a reproducible authenticated local profile that provisions development-only control credentials through a local secret file and exercises the same credential checks as `serve`. Reuse the existing server/storage stack and defaults; do not introduce a second persistence implementation just for the tutorial.

The main tutorial should reach a useful result through the supported client, then show how Git joins the same history. Keep operator configuration and storage architecture available as separate follow-on material. A small connection diagnostic should report dependency reachability, authentication, and the returned remote origin; the existing harness doctor is a starting point.

Acceptance: a new teammate can start a clean environment and run a permission-aware create/write/read/revoke example without assembling configuration from several pages.

**9. Standardize the ordinary surface without a disruptive route rewrite.**

Today namespace/repository lists use cursors, credentials use pages, and history uses offsets. Some successful empty lists are `null`. Commit identifiers appear as `sha`, `hash`, and `commit`; credential plaintext has two names. Repositories return IDs but ordinary REST routes require account/namespace/name. `/file`, `/raw`, and `/blob` offer related content access with different addressing and response behavior. See [the API reference](../docs/core/api-reference.md) and [wire types](../internal/types/git.go).

Normalize empty collections to `[]`, use one pagination convention for new collection APIs, and choose consistent identifier names in the client. Give repository handles a canonical locator so callers do not manually rebuild routes. Add ID-based lookup only if teams need durable addressing across future naming changes; a second complete route tree is not necessary just to shorten URLs.

Make `/file?ref=&path=` and `/tree?ref=&path=` the canonical application reads. Retain object-hash endpoints as advanced Git tools and WAL as diagnostics. Raw-byte success responses are appropriate for files; wrapping all content in JSON would add encoding and memory costs. The client should make that distinction routine.

The `/client/v4/accounts/.../artifacts` prefix is verbose, but hiding it behind client configuration has much more immediate value than renaming it. Preserve existing route aliases and envelopes while introducing improvements. Defer breaking route, field, and status-code changes to a documented version boundary.

**10. Make version pinning convenient, including handoffs.**

Tree reads already return the resolved commit, and file reads already accept a SHA. That is the right foundation. Currently the caller must propagate it manually; file responses themselves do not expose the resolved commit. Fork input selects a repository's captured publication and optionally its default branch, but does not accept a caller-selected commit. See [tree reads](../internal/api/content_tree.go), [file reads](../internal/api/content_file.go), [fork input](../internal/types/repo.go), and [fork implementation](../internal/jobs/fork.go).

Expose a client version handle that resolves a branch once and pins every subsequent read. Add resolved-commit metadata to file responses, keeping blob identity and commit identity distinct. Then evaluate a fork-from-version operation for teams that need to continue exactly the result they reviewed. This requires reconciling requested commits with snapshot lineage and retained objects; it is more than adding a request field.

Acceptance: a manifest and its outputs cannot silently come from different publications when using a version handle. A version-pinned handoff remains exact even if the source branch advances.

**11. Add binary writes only through the same publication contract.**

REST accepts string contents; PDFs, images, executable modes, and larger bundles require Git. For a small generated binary file, installing and managing a Git working copy can be disproportionate. See [write limits](../docs/core/writing-files.md) and [commit file types](../internal/types/storage.go).

Start with a bounded byte-content representation for atomic changes, choosing explicit base64 or multipart based on the first consumer's needs. Define decoded-byte limits, content encoding, and file modes. Large staged uploads can follow demonstrated demand; they introduce expiry, orphan cleanup, and separate upload/publication states. Preserve Git as the established route for large trees and normal merges.

Acceptance: a worker can publish a small report and image together and read back the original bytes at one SHA. Uploaded bytes must not become visible as a partial result before the commit publishes.

**A concrete target integration**

The following is illustrative proposed client syntax, not an existing API. Repository creation is idempotent and seeded; credentials are explicit; updates are incremental and conditional.

```ts
const artifacts = new Artifacts({ url, account, namespace, controlToken });
const repo = await artifacts.repos.create({
  name: `run-${runId}`,
  files: [{ path: "inputs/brief.md", text: brief }],
  idempotencyKey: `create-${runId}`,
});
const access = await repo.credentials.create({ scope: "write", ttl: 3600 });
// Persist access.id for revocation; pass the secret only to this worker.

const workerRepo = Artifacts.repository(repo.locator, access.plaintext);
const base = await workerRepo.version("main");
const published = await workerRepo.commitChanges({
  branch: "main",
  expectedHead: base.commit,
  writes: [{ path: "outputs/report.md", text: report }],
  deletes: [],
  idempotencyKey: `report-${runId}-attempt-${attemptId}`,
});

// The backend validates this exact publication before marking its run complete.
const result = await repo.at(published.commit).files.readText("outputs/report.md");
await repo.credentials.revoke(access.id);
```

The idempotency key is stable across transport retries of the same request; a changed report is a new intended publication with a new key. A conflict requires reconciliation, not an automatic retry against a newly fetched head. The application's completion state and output validation remain application responsibilities.

**Recommended delivery sequence**

1. Fix validation/conflict mappings, expose initial credential metadata, and normalize empty lists. Specify the write precondition, idempotency, and permission contracts before growing the interface.
2. Deliver atomic changes with expected-head checks and idempotent publication, then repository-scoped REST access. Validate the complete delegated-worker flow, including intervening Git writes and lost responses.
3. Add idempotent seeded creation, the supported client/schema, and authenticated local setup. Rewrite the primary integration example around this flow.
4. Improve operation lookup and import recovery. Add pinned forks, binary REST publication, or notifications when an adopting team's workflow demonstrates the need.

Keep native Git, immutable commit reads, repository isolation, and explicit snapshots. Avoid expanding this pass into project management, application completion state, cross-repository search, REST merge orchestration, or a general permissions language. Those would add substantial concepts before the ordinary file workflow is simple.

For the supporting UI, move WAL and file modes behind diagnostics, replace the storage-focused landing copy with repository/file language, and add actionable empty states and copyable API/commit references. These observations come from the [template](../internal/ui/templates/page.html) and [documented behavior](../docs/developer-tools/web-ui.md), not a rendered visual inspection. They are secondary to the developer contract.

Evaluate the changes with concrete tasks: first authenticated publication; one-file update without resending inputs; delegated REST read/write; concurrent-writer conflict; recovery from a lost response; exact-version readback; and credential revocation. Record completion time, bespoke integration code, and recovery steps against today's baseline. This review supplies the hypotheses; those tasks establish whether the interface actually became easier.
