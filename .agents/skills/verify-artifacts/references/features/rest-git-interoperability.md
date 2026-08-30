# REST and Git interoperability

## Scope

REST commits and Git pushes publish through the same immutable pack WAL and
Postgres ref authority. Content written through either interface must be
visible through the other at the same published commit.

## User paths

All REST paths are below
`/client/v4/accounts/{account}/artifacts/namespaces/{namespace}/repos/{repo}`.

| Method | Path | Behavior |
|---|---|---|
| `POST` | `/commits` | Publish a bounded file snapshot to a branch |
| `GET` | `/file?ref={ref}&path={path}` | Stream a file as `application/octet-stream` |
| `GET` | `/raw/{ref}/{path}` | Stream a file with detected content type |
| `GET` | `/log?ref={ref}&limit={n}&offset={n}` | List commit history |
| `GET` | `/commit/{sha}` | Read parsed commit metadata |
| `GET` | `/tree/{sha}` | List entries in a tree object |
| `GET` | `/blob/{sha}` | Stream a blob |
| `GET` | `/refs` | List authoritative published refs |
| `GET` | `/wal` | List immutable published pack records |

Git uses the remote returned at repository creation:
`{public_url}/git/{account}/{namespace}/{repo}.git`.

## Core observable proof

Use a unique repository and unique file values for every run.

1. Create the repository over REST.
2. `POST /commits` with a bootstrap file. Require HTTP `201`, a non-empty SHA,
   and sequence `1`.
3. Read the file through `/file` and require exact bytes. Require `/refs` to
   contain the default branch at the REST commit SHA and `/wal` to contain its
   publication sequence.
4. Clone the returned remote with real Git and require the checked-out bootstrap
   file and `HEAD` SHA to match REST evidence.
5. Add a unique result file, commit locally, and push it to the default branch.
6. Read the result through REST and require exact bytes. Require refs to point
   to the Git commit and WAL to advance.
7. Preserve request outcomes, Git stdout/stderr, SHAs, sequences, file digests,
   refs, and WAL summaries. Redact credentials before writing evidence.

## Error-path proof

- Reject an empty file list, more than 100 files, duplicate paths, `.git`
  paths, parent traversal, invalid branches, and more than 1 MiB of content.
- Reject JSON larger than 2 MiB, unknown fields, malformed JSON, and trailing
  JSON values.
- A failed publication must not advance authoritative refs or WAL sequence.
- Missing refs, commits, trees, blobs, or paths must fail rather than returning
  successful empty content.

## Gotchas

- `files` is the desired committed snapshot. The implementation loads the
  parent index and runs `git add -A`; a prior path omitted from a later request
  is removed.
- Empty branch selects the repository default. Empty message, author name,
  author email, and date receive deterministic product defaults except for the
  current timestamp.
- `/file`, `/raw`, and `/blob` stream bytes rather than a JSON envelope. Do not
  attempt to decode their successful bodies as Cloudflare responses.
- Ref resolution accepts `HEAD`, a 40-character object SHA, a full ref, a branch
  name, or a tag name.
- Log defaults to 20 entries and offset 0.
- Git receive input is capped at 512 MiB. Git and REST streams also use the
  configured inactivity timeout.
- Success is not proved by matching only a response SHA. Require the final
  cross-interface file bytes, authoritative ref, and WAL sequence.

## Source anchors

- Content route surface: `internal/api/server.go:67-78`
- REST commit bounds and publication: `internal/repository/commit.go:14-67`
- Snapshot construction and defaults: `internal/repository/commit.go:100-155`
- Ref resolution: `internal/api/content.go:25-51`
- File streaming: `internal/api/content_file.go:12-36`
- Git receive and WAL publication: `internal/repository/publish.go:18-64`
- End-to-end drive: `examples/agent-harness/drive.go:52-159`
- Storage publication ordering: `docs/storage.md:5-14`
