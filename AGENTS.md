# Working on Artifacts

Read [GOALS.md](GOALS.md) for product intent, then use this file to locate the relevant implementation and checks. Keep explanations in their owning document and link to them rather than copying large sections here.

## Find the right context

| Task | Start here |
|---|---|
| Run the service | [README.md](README.md), [configuration](docs/core/configuration.md) |
| Change developer interfaces | [API reference](docs/core/api-reference.md), [application integration](docs/core/integration.md), `internal/api/`, `internal/service/` |
| Change REST commits or Git behavior | [writing files](docs/core/writing-files.md), [Git](docs/core/git.md), `internal/repository/`, `internal/githttp/` |
| Change publication, reads, or compaction | [storage architecture](docs/storage.mdx), `internal/repository/`, `internal/store/` |
| Change authentication or delegation | [authentication](docs/core/authentication.md), `internal/auth/`, `internal/api/auth.go` |
| Change forks, imports, or deletion | [forks and imports](docs/core/forks-and-imports.mdx), `internal/jobs/` |
| Change browser tooling or documentation | `internal/ui/`, `docs/`, `docs-site/` |
| Verify a running instance | [verify-artifacts skill](.agents/skills/verify-artifacts/SKILL.md) |

Reviews and saved evidence describe a particular revision and run. Treat them as historical context, not as a replacement for current code and contracts.

## Decisions (append-only)

Keep the decision register in this document, ordered D1, D2, D3, and onward. IDs are permanent: never renumber, reorder, delete, or reuse them. Append each new decision after the last entry in this section.

Once a decision is recorded, never edit its text or its supporting decision files. Changes, corrections, reversals, and new evidence go in a new numbered decision. State `Supersedes: D<N>` or `Amends: D<N>` in the new entry, identify exactly what changes, and leave the earlier entry untouched. Even a superseded label belongs in the new entry, not as an edit to the old one. Later decisions take precedence only over the points they explicitly amend or supersede; unaffected decisions remain in force.

Each entry includes its ID, recording date, title, chosen behavior, and rationale. Include validation, limitations, and links when they help assess the decision. If a separate decision file is needed, give it the same ID, link it from the entry, and treat it as immutable too. Keep current API and operational documentation up to date separately from these historical records.

The initial entries below record the existing baseline on 2026-09-19. Their dates are recording dates, not claims about when the original architecture was chosen.

### D1 (2026-09-19): Postgres owns publication

Postgres is the publication authority. Object storage holds immutable Git data; local bare repositories, indexes, and range caches are disposable. Store the immutable data before committing publication metadata and ref transitions. A successful response must not expose refs whose required data is not durable. This keeps acknowledged history independent of local cache survival.

### D2 (2026-09-19): REST and Git share one history

REST commits and Git pushes use the same repository history and publication mechanism. Do not introduce an independent REST file store or a second publication authority. Applications and ordinary Git clients must be able to exchange work through either interface.

### D3 (2026-09-19): File changes are incremental and retries are explicit

REST file writes preserve untouched paths by default. Deletions and full replacement are explicit. Preserve expected-head checks and transactional idempotency so clients can detect intervening writes and recover supported operations after lost responses. An uncertain transport outcome is not proof of rollback. See [writing files](docs/core/writing-files.md).

### D4 (2026-09-19): Delegated credentials are repository-scoped

Control-plane credentials manage account resources. Repository credentials cover their repository's permitted content/Git operations, not account management. Check binding, scope, expiry, revocation, and read-only policy at the appropriate boundaries. Workers should not need account-wide credentials to work with their repository. See [authentication](docs/core/authentication.md).

### D5 (2026-09-19): Forks preserve a captured history boundary

Fork history is bounded by the captured parent publication. A newer parent checkpoint must not move that boundary. Compaction must preserve the objects needed by supported reads and forks; pack deletion or retention changes need their own decision and evidence. This allows independent work without making a fork follow future parent changes.

### D6 (2026-09-19): Publish incremental packs and compact separately

Writes publish self-contained incremental packs. Background compaction creates checkpoints separately from publication requests. Content reads use indexes and remote byte ranges where supported, with a disk-backed fallback for large objects. Git serving and writes still use a full local cache. The intent is to avoid republishing or reading unrelated data; performance claims require measured workloads. See [storage improvements](reviews/storage-improvements.md).

### D7 (2026-09-19): Events describe committed publications

Publication events come from committed records. Preserve complete publications, resumable cursors, and duplicate-tolerant consumption. Compaction does not create a publication event. Do not claim exactly-once application execution or treat publication as proof that a business task succeeded. See [the API reference](docs/core/api-reference.md).

### D8 (2026-09-19): Support one serving node

The supported service topology is a single serving node with Postgres and durable object storage. Cross-process cache locks and database transactions do not by themselves establish multi-node correctness. Additional serving topologies require an explicit design and verification before being described as supported.

### D9 (2026-09-19): Keep prose comments out of code

Never write prose comments in implementation files, tests, examples, or documentation-site source. Express intent through names and structure, with explanations in Markdown when needed. Retain only machine-interpreted compiler, build, or static-analysis directives, such as `//go:embed` and `//nolint:gosec`; do not append prose explanations to them.

### D10 (2026-09-19): Change decisions by adding new decisions

The numbered register and its supporting decision files are append-only. Change direction by appending an amendment or superseding decision, never by rewriting an earlier record. This preserves the reasoning available to future contributors and makes architectural changes visible in review. The next decision after this baseline is D11.

### D11 (2026-09-19): Verify the pushed revision in CI

After an authorized push, inspect the CI runs for that exact commit and wait for the relevant checks to finish before reporting completion. Local verification and remote CI are separate evidence; a cached dependency can let local checks pass while a fresh runner fails. If remote results cannot be inspected, explicitly report CI as unverified. Investigate existing failures before attributing them to the new change, and carry authorized fixes through a successful remote run. This addresses the missed MinIO image-pull failures in [CI run 8](https://github.com/mikerudolph/artifacts/actions/runs/35480748626).

## Working rules

1. Trace the current behavior and relevant tests before editing. Keep existing user work intact. Carry authorized work through implementation and verification; surface material ambiguity without stopping independent work.
2. Prefer clear names, small functions, and direct control flow. Add abstractions when they reduce repeated responsibility or establish a real boundary.
3. Follow D9: **never write prose comments in code**. Keep explanations in Markdown and retain only machine-interpreted directives.
4. Follow the existing formatting, lint, complexity, and coverage gates. Refactor or fix failures rather than weakening checks or adding broad suppressions to make a change pass.
5. Bound request bodies, buffering, storage transfers, and background work. Respect cancellation and existing transfer limits. Keep expensive maintenance outside publication requests.
6. For storage changes, report the effect on transferred bytes, storage requests, and local materialization. Use measured fixtures when claiming an improvement; a small upload does not establish low CPU cost or high throughput.
7. Preserve supported public behavior unless the task authorizes a change. When semantics change, update callers, tests, examples, and migration guidance together. Do not invent a blanket compatibility policy.
8. Keep public docs and configuration descriptions aligned with the implementation. Label targets, limitations, and deferred work explicitly.
9. Use task-owned databases, storage directories, and server processes for live tests. Do not modify a developer's global Git configuration or stop unrelated services. Keep credentials out of command arguments, output, and evidence.
10. Report what changed, which checks ran, and any remaining limits. Distinguish implemented behavior from measured behavior and from assumptions.

## Verification

Start with the narrowest useful checks while developing. Add regression tests for changed behavior and failure modes; do not add tests that merely repeat the implementation or inflate coverage.

Before completing code changes, run:

```sh
make verify
```

The [Makefile](Makefile), [.golangci.yml](.golangci.yml), and [.testcoverage.yml](.testcoverage.yml) own the exact checks and thresholds. They currently cover formatting/imports, vet, lint, race tests, coverage, and Go file-size limits. Do not duplicate their configuration here.

For changes to public REST/Git behavior, storage durability, authentication, or recovery, also use the [verify-artifacts skill](.agents/skills/verify-artifacts/SKILL.md). Run its preflight and real REST → Git → REST workflow on an owned instance. Preserve redacted evidence and confirm cleanup. Live verification complements `make verify`; neither replaces the other.

For documentation-site or documentation changes that affect the site, run from `docs-site/`:

```sh
npm run check
```

Once relevant checks pass, repeat or broaden them only when another change, a failure, or an unresolved concern warrants it. If a required check cannot run, name the blocker and report the checks that did run; do not present that as a pass.

## Recording the next decision

Before changing an established decision, read the register and any later amendments. Append the next unused D-number in the register above, with a date and explicit links to any decisions it changes. Record the problem, chosen behavior, tradeoff, and validation or unresolved limits. Put any longer supporting record in a new file such as `reviews/decisions/D11-short-title.md` and link it from D11. Never update a past decision file to reflect the new choice.

Update the current contract documentation and implementation to reflect the resulting behavior. These working documents can evolve; historical decision entries and their supporting files cannot.

Keep [GOALS.md](GOALS.md) focused on product direction. Add a decision when it establishes or changes a meaningful architectural or working rule, not for every implementation detail.
