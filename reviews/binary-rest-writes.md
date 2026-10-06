# Binary REST writes for large commits

Proposed 2026-10-05 against `f59a41024dd49e4e9e044695206d7e2e49bc5657`. The user clarified that individual files and complete commits should support tens or hundreds of MiB. This updates the small-payload recommendation for request 2 in the [integration assessment](integration-requests.md); the [original issue snapshot](sources/github-issue-1.md) remains unchanged.

The proposal below records the design before implementation. [D14](../AGENTS.md#d14-2026-10-05-stream-rest-file-content-before-atomic-publication) now records the adopted architecture; [writing files](../docs/core/writing-files.md) owns the current contract. Implementation refinements make declared sizes optional for generated streams and immediately unlink temporary upload files while their handles remain open, so process exit releases them without a stale-directory collector. JSON retains its 1 MiB content and 2 MiB body limits.

## Recommended contract

Add `multipart/form-data` to the existing repository `POST /commits` route. Send one small JSON manifest followed by raw file parts. Stream the files to request-owned disk storage, then publish every write and deletion together through the existing Git history. Return the existing HTTP 201 result containing `sha` and `sequence` only after durable publication.

Multipart describes how several fields and files share one HTTP body. It carries binary bytes directly. The boundary separates parts; the client library should generate it. See [RFC 7578](https://www.rfc-editor.org/rfc/rfc7578.html). This proposal uses one synchronous request. It does not introduce independently retriable chunks, an upload session, direct-to-S3 uploads, or a separate REST file store.

Keep the existing JSON request and limits for small text changes. The new large-file route requires multipart; increasing the JSON cap does not provide streaming. JSON/base64 is deferred: the issue proposed that particular representation, while this proposal satisfies its mixed binary/text and executable-mode workflow through multipart. Do not describe the exact JSON encoding request as implemented when closing the issue checklist.

## Request shape

The first part is named `manifest`, has `Content-Type: application/json`, and contains the existing commit metadata plus file descriptors:

```json
{
  "branch": "main",
  "expected_head": "0123456789abcdef0123456789abcdef01234567",
  "message": "Publish report",
  "files": [
    {"path": "outputs/report.md", "part": "f0", "size": 9},
    {"path": "outputs/report.pdf", "part": "f1", "size": 104857600},
    {"path": "scripts/run.sh", "part": "f2", "size": 10, "mode": "100755"}
  ],
  "deletes": ["outputs/draft.md"]
}
```

The SHA is illustrative; clients supply their actual observed head. The subsequent `form-data` parts are named `f0`, `f1`, and `f2`. Their bodies are respectively `# Report\n`, a 100 MiB PDF, and `echo done\n`. Send `Idempotency-Key` as the existing HTTP header. A caller can supply the existing `author`, `base`, and `replace` fields in the manifest.

| Element | Proposed behavior |
| --- | --- |
| `path` | Existing normalized repository-relative path rules; the manifest is the only source of the destination path. |
| `part` | Unique ASCII identifier matching `[A-Za-z0-9_-]{1,64}`, excluding `manifest`. Exactly one file part must match each descriptor. |
| `size` | Required nonnegative integer byte count. Check the declared aggregate before receiving files, and verify each actual byte count while streaming. Zero-byte files are valid. |
| `mode` | Optional string `100644` or `100755`. Omission preserves an existing executable's mode; otherwise the existing regular-file default applies. Explicit mode can change it. |
| File bodies | Raw bytes for both text and binary. Parts may follow the manifest in any order. No inline `content` or `encoding` field in a multipart descriptor. |
| Filenames and media types | A part's `filename` is optional and never becomes a filesystem path. File media types are advisory and are not stored as Git metadata. |
| Commit behavior | Preserve untouched paths, exact-path deletion, explicit replacement, branch defaults, `base`, and optional `expected_head` semantics. Preserve current symlink replacement rules. |

Reject unknown manifest fields, duplicate JSON member names, invalid paths/modes, duplicate descriptors or part names, missing/extra parts, size mismatches, and malformed or unfinished boundaries. Require one JSON value. Reject nested multipart file parts and transfer encodings; no request decompression in this first version. File bytes must reach Git unchanged. A deletion-only or empty replacement manifest needs no file parts and remains subject to the existing commit validation.

Dispatch well-formed multipart Content-Type to the new parser. Preserve the existing JSON decoding path for legacy requests, including clients that omit Content-Type; do not accidentally tighten unrelated management routes. Keep transport parsing separate from shared commit validation.

## Initial resource limits

These are proposed starting defaults to validate, not advertised capacity. Make the multipart byte limit, active-request limit, and total deadline configurable with finite bounds. Keep configuration scoped to commits.

| Limit | Proposed default | Purpose |
| --- | --- | --- |
| Aggregate file content | 512 MiB per commit | Supports one large file or several smaller files; a single file cannot exceed this total. |
| Complete HTTP body | Content limit plus 2 MiB | Bounds manifest, MIME headers, framing, and other wire overhead; 514 MiB at the default. |
| Manifest | 256 KiB | Keeps metadata parsing bounded independently of file content. |
| Changes | 100 writes plus deletions combined | Preserves the existing count limit. At most 101 MIME parts including the manifest. |
| Active multipart requests | 2 per serving process | Hold admission from before body parsing through publication and cleanup. At the default, live raw upload staging is bounded by 1 GiB. |
| Ingress idle timeout | Existing 30-second stream timeout | Stop stalled reads while allowing continuing progress. |
| Total operation deadline | 15 minutes | Bounds receipt, waiting for cache ownership, preparation, upload, and publication. Validate against cold-cache fixtures. |
| Legacy JSON | Existing 2 MiB wire / 1 MiB content | Existing clients keep their current contract. |

Reject excess concurrent multipart requests with 503 and `Retry-After`, rather than accepting their bodies into an unbounded queue. Per-process admission is an initial resource bound, not an account-wide quota. Each independent serving instance has its own budget. Account fairness can be added separately if required.

Use the streaming [`Request.MultipartReader`](https://pkg.go.dev/net/http#Request.MultipartReader), with [`NextRawPart`](https://pkg.go.dev/mime/multipart#Reader.NextRawPart) so the parser does not silently decode quoted-printable data. Do not use `ParseMultipartForm`, `FormFile`, or whole-file reads. Explicitly count parts: `ReadForm`'s part-count limit does not establish the limit for a streaming reader. Audit and test the pinned Go version's MIME header byte/count bounds as part of the memory budget; the file limit alone does not bound parser allocation.

Enforce limits using actual reads even without Content-Length. Stop at the first excess byte without consuming an unbounded remainder. Account for preamble/epilogue and framing within the complete-body bound before preparing a commit. Copy one part at a time with a fixed-size buffer, checking cancellation and computing SHA-256 as bytes are written. The declared size supports admission but is never proof of received content.

Extend the existing [stream deadline helper](../internal/httpstream/stream.go) so each read deadline is the earlier of its idle deadline and the total deadline. Ensure cancellation interrupts a blocked body read; a context deadline alone is insufficient. Make repository lock acquisition cancellable as well. Apply cancellation to local copies and child Git commands, including the S3 backend's existing upload-staging copy.

## Disk ownership and cost

Create one private request directory with generated names, directory permissions 0700 and file permissions 0600. Store uploaded bytes there, never in a client-derived local path. Remove it on success, validation failure, cancellation, or publication failure. For crash leftovers, use owned spool roots and live-owner locking to distinguish abandoned directories from active requests; do not delete another process's files merely because they are old. Bound cleanup work and finish necessary stale-spool recovery before admitting new uploads.

Use disk-backed scratch storage in the container's writable volume. A tmpfs moves this cost into memory. Temporary uploads are disposable: they need not survive a crash or be fsynced as a durable upload session. Git object data must still reach durable object storage before Postgres publishes refs under D1.

The raw-content limit is not a total disk quota. Peak local usage also includes the full repository cache, new Git loose objects, a generated pack/index, and the [S3 backend's second staging copy](../internal/store/object/s3/s3.go). Two 512 MiB requests can therefore need substantially more than 1 GiB of free disk. Admission bounds upload staging; deployment volume sizing and failure handling must cover the complete path. Disk exhaustion returns a retriable capacity error and must not expose a partial commit.

Multipart removes base64 representation costs, but the current write path still materializes a full Git cache. It still generates and uploads an incremental pack and index, and cache reconstruction can download unrelated history. Record request bytes, object-store requests and bytes, temporary/cache disk peaks, Go memory, child Git memory, CPU, and elapsed time. Streaming ingress alone establishes none of the other performance bounds. Internal S3 transfer optimization can be evaluated separately from the client request format.

## Publication and retries

The current keyed [commit path](../internal/repository/commit.go) holds the repository cache lock and a database idempotency transaction across Git preparation and object uploads. At hundreds of MiB, changing only the parser would leave long database transactions and slow clients competing for cache ownership.

Use this sequence:

1. Authenticate, resolve the repository, validate the idempotency key, and obtain multipart admission. Parse the bounded manifest and stage every file. No repository cache lock or long-lived database transaction is held during receipt.
2. Revalidate access after the upload and compute a request digest from the validated manifest and server-computed file hashes. Look up a completed idempotency result before checking the current head. A completed matching retry returns the original result even after the branch advances.
3. Acquire the cancellable repository cache lock, repeat completed-result lookup as needed, and capture the current repository/refs snapshot. Check the existing lifecycle, branch, expected-head, path-collision, and mode rules. Stream the staged files into Git, prepare the commit, and upload its immutable pack and index. Retain exclusive local cache ownership through finalization.
4. Revalidate authorization immediately before finalization. In a short metadata transaction, serialize the existing scope/key, check its result again, and either replay it or publish with the captured repository sequence and expected ref. Store the result in the same transaction as publication. No Git command, file hashing, or object transfer belongs inside this transaction.
5. Reconcile or evict local cache state according to the result actually published, release the cache lock, delete staging, and return the existing response. If another instance won the same key, its commit may differ from this attempt's prepared candidate; never mark the unused candidate as published locally.

Keep authorization checks ahead of cached-result disclosure. Expired or revoked repository credentials and deleted repositories cannot obtain content access by retrying. Carry a safe principal identity through preparation rather than persisting a bearer token. Test expiry/revocation completed during upload and before final authorization. This proposes additional checks for long requests; strict serialization of a concurrently executing revocation with the publication transaction would be a separate, explicitly specified guarantee.

Moving preparation outside the transaction allows duplicate preparation on independent instances. This is the tradeoff for avoiding database locks and connections across large transfers. The transactional idempotency check still permits only one publication/result. If preparation fails because another instance advanced the head, check for a now-completed keyed result before returning a head conflict; that advance may be the same operation succeeding elsewhere.

Uploaded but unpublished packs may remain after conflicts or failures, as they can today. Track this cost in concurrency fixtures. Do not add unsafe age-based object deletion to this feature; reference-aware orphan cleanup belongs to request 1. Publication still uses the existing repository-wide sequence conflict behavior and performs no automatic merge.

### Digest rules

Add a prepared-commit representation with reopenable content sources, sizes, and hashes. The wire DTO should not become a giant byte slice or JSON-marshaled file body. Add metadata helpers for completed-result lookup and execution with a precomputed digest, retaining the existing commit/account/repository key scope.

For multipart, hash a versioned canonical representation containing submitted commit metadata, normalized file paths, explicit/omitted mode, verified sizes, server-computed SHA-256 hashes, and deletions. Sort file descriptors and deletions for this new representation. Exclude MIME boundaries, part identifiers/order, filenames, content-type headers, and temporary paths. Preserve meaningful distinctions such as absent `expected_head` versus an empty value. Hash omitted author time as the submitted default sentinel, never a freshly generated timestamp; do not resolve a moving branch default into the digest.

Keep the legacy JSON digest byte-for-byte compatible with the current [decoded-request serialization](../internal/store/meta/idempotency.go), including field omission and ordering rules. A new transport tag/domain distinguishes multipart digests. Reusing one key across JSON and multipart conflicts; clients retry using the same transport and operation. No database migration appears necessary because the existing digest column is text, but regression fixtures must prove pre-upgrade JSON replay.

### Client recovery

An interrupted upload restarts from the beginning. A keyed retry resends the full manifest and files so the server can verify the operation before replaying its result; there is no key-only receipt endpoint in this version. Use a new key only when the intended operation changes.

A lost response is an uncertain outcome: publication may already have committed. A matching retry must return the original SHA/sequence with no duplicate event or commit publication. Without a key, clients must inspect state before repeating the operation. Recommend both `Idempotency-Key` and `expected_head` for application writes while preserving their current optional status.

Return 400 for invalid structure, 413 for configured size/count excess, the existing 401/403 for authorization, and the existing 409 kinds for head/idempotency conflicts. Use 503 with `Retry-After` for admission or scratch capacity failures. Document deadline and transport failures as potentially uncertain once finalization is attempted. Do not describe every failure response as proof of rollback.

## Smoothest implementation sequence

| Delivery | Concrete change | Evidence before proceeding |
| --- | --- | --- |
| 1 Shared preparation | Separate wire inputs from prepared commit content; support reopenable file sources and optional regular/executable mode without materializing file bodies in strings. Keep legacy JSON validation and digest serialization stable. | Existing write/mode tests, arbitrary-byte source tests, failure cleanup, and a frozen legacy replay fixture. |
| 2 Short publication transaction | Separate Git/object preparation from transactional publication; add completed-result lookup and precomputed digest support; make cache waiting cancellable. | Independent-cache competing writes, same-key races, changed-body conflicts, replay after branch advancement, unused-candidate eviction, and failures around durable upload/metadata commit. |
| 3 Multipart endpoint | Add manifest parsing, bounded streaming spool, admission/deadlines, authorization rechecks, crash cleanup, errors, configuration, and streaming client examples. | Malformed/bounded HTTP fixtures, auth/cancellation tests, exact mixed-file REST/Git round trips, docs checks, and no whole-file client buffering. |
| 4 Size and recovery qualification | Run representative full-size workloads in the built image on owned resources, then publish measured limits and operational guidance. | 100 MiB file and 512 MiB aggregate fixtures, two instances, restart/cache loss, slow clients, disk pressure, and recorded resource/transfer evidence. |

Keep regression tests and required verification in each delivery; the last delivery adds capacity evidence rather than postponing correctness testing. Each code change requires `make verify`. Public REST/Git, authentication, durability, and recovery changes also require the [verify-artifacts workflow](../.agents/skills/verify-artifacts/SKILL.md) on owned resources. Update [writing files](../docs/core/writing-files.md), the [API reference](../docs/core/api-reference.md), [configuration](../docs/core/configuration.md), errors documentation, and client examples when behavior is implemented, then run `npm run check` in `docs-site/`. Extend OpenAPI if that work has landed; it need not block this feature.

Record an adopted architectural choice as the next unused decision before implementation changes the established write path. The next ID at this inspected revision is D14. Cover transport scope, resource ownership, and preparation outside the idempotency transaction while preserving D1/D2/D3/D12. Keep this working proposal editable; any later supporting decision record must be a separate immutable file.

## Acceptance matrix

| Area | Required cases |
| --- | --- |
| Bytes and modes | Mixed Markdown, arbitrary bytes including NUL, a large incompressible file, empty file, explicit executable/regular mode, preserved untouched binary/executable paths, existing symlink rules. Read exact bytes by SHA and clone with Git. |
| Commit semantics | Incremental edits, delete-only, empty replacement, base/new branch, stale expected head, concurrent Git push, and repository-wide sequence conflicts. One publication contains all requested changes. |
| Input bounds | Manifest first, strict JSON, traversal/duplicates, unknown/missing/extra parts, invalid boundaries, oversized MIME headers, wrong declared sizes, excessive part counts, exact-limit/one-byte-over cases, and chunked requests without Content-Length. |
| Cancellation and cleanup | Stalled and slow-progress uploads, total deadline, waiting for cache ownership, cancellation during local/S3 staging, full disk, admission exhaustion, process death, and ownership-safe stale-spool recovery. |
| Authorization | Cross-repository credentials, read-only scope/policy, the existing initial control-write exception, expiry/revocation during upload, deleted repository, and denial before replay disclosure. |
| Retry and race behavior | Same bytes with a different MIME boundary/order replay; changed bytes or metadata conflict; restart and branch advancement preserve replay; two instances return one result with one publication; legacy JSON records still replay. |
| Durability | Crash during receipt, after object upload/before metadata, and after publication/before response. Remove independent local caches and reconstruct through real REST → Git → REST. |
| Capacity evidence | Disk-backed generated data and actual HTTP/storage transfers, cold/warm caches, two simultaneous uploads, both compressible and incompressible content. Measure the Go process and Git children. Avoid memory-backed test stores or payload-sized test buffers when claiming bounded memory. |
| Deployment | Non-root image with read-only root filesystem and owned writable volumes; intermediary body/time limits and buffering verified; bounded shutdown may interrupt uploads, and retry remains the recovery mechanism. |

## Proposal validation

The request, commit/tree, metadata idempotency/publication, authentication, HTTP deadlines, local cache, Git execution, and S3 staging paths were inspected. Existing interface, mode, multi-instance, request-bound, and stream-timeout tests provide starting coverage; they do not establish the proposed behavior. Primary multipart and Go streaming references are linked above.

At proposal time, only review documents changed. Local links, the JSON example and declared small-file sizes, whitespace, and all 13 assessment sections passed document checks. The saved issue snapshot still matches the original exactly. Those proposal checks did not establish implementation, live behavior, or capacity.

## Implementation verification (2026-10-05)

The working tree implements multipart commits, optional declared sizes, executable modes, private unlinked spooling, bounded admission and deadlines, authorization rechecks, canonical multipart retries, and preparation outside the metadata transaction. Streaming Node, Python, and curl examples are in [binary-commits](../examples/binary-commits/README.md). The [verification skill](../.agents/skills/verify-artifacts/SKILL.md) includes a binary drive and isolated container reproduction command.

The [first live run](evidence/multipart-2026-10-05/binary-512/report.md) exposed an idle read deadline that remained active during publication. A real HTTP regression reproduced that cancellation after receipt; clearing the read deadline after parsing fixed it while preserving the total operation deadline. The failed evidence remains available. The corrected image passed:

- [100 MiB aggregate content](evidence/multipart-2026-10-05-pass2/binary-100/report.md) and [512 MiB aggregate content](evidence/multipart-2026-10-05-pass2/binary-512/report.md), with incompressible binary bytes plus text and an executable. Each drive passed 17 assertions, including SHA-pinned checksums, Git fetch/edit/push, keyed replay after advancement, conflicts, exactly four publications, and repository cleanup.
- [Container checks](evidence/multipart-2026-10-05-pass2/checks.json): streaming Node and Python examples, unknown-size input, two independent serving instances, competing writes, cross-instance replay, cold-cache restart, bounded shutdown, and removal of owned containers, volumes, network, and client scratch. All 48 recorded checks passed, including final cleanup.
- [make verify](evidence/multipart-2026-10-05-pass2/make-verify.txt): formatting, vet, lint, race tests, coverage gates, and Go file-size limits. Total coverage was 89.8% (4040/4500 statements).
- [Documentation checks](evidence/multipart-2026-10-05-pass2/docs-check.txt) and skill frontmatter validation passed. Runtime [source hashes](evidence/multipart-2026-10-05-pass2/source.json) and [image identity](evidence/multipart-2026-10-05-pass2/image.json) identify the tested uncommitted revision.

The successful 512 MiB multipart publication took 40.062 seconds in this local Docker run while repository checks were also running. This is a fixture observation, not a throughput guarantee. The binary file was 536,870,865 bytes; the text and executable brought aggregate content to exactly 536,870,912 bytes. Both live drives used real Postgres and MinIO with disk-backed cache volumes.

Ingress now carries raw bytes without base64 expansion and adds one disk spool per file before Git preparation. The existing publication path still writes an incremental pack and index to object storage; S3 still stages each upload and performs its immutable-object checks. Concurrent same-key preparation on independent instances can upload unused candidate packs. Local Git materialization and backend staging remain additional disk costs. Peak application/child-Git memory, disk peaks, object-store request counts, and transferred object bytes were not instrumented, so no improvements in those quantities are claimed. This run did not inject process death at publication boundaries, run simultaneous full-size uploads, or exercise a production proxy. Upload sessions, resumability, and orphan-pack collection remain deferred.
