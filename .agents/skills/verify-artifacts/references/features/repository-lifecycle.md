# Repository lifecycle

## Scope

Accounts are tenants, namespaces group repositories, and repository names are
unique within a namespace. Creating a repository implicitly ensures its account
and namespace, creates symbolic `HEAD`, transitions the repository to ready,
and returns a tenant-qualified Git remote plus an initial write credential.

## User paths

All REST paths are below
`/client/v4/accounts/{account}/artifacts`.

| Method | Path | Behavior |
|---|---|---|
| `POST` | `/namespaces` | Create a namespace with optional `eu` or `us` jurisdiction metadata |
| `GET` | `/namespaces` | List namespaces with `cursor` and `limit` |
| `GET` | `/namespaces/{namespace}` | Read one namespace |
| `POST` | `/namespaces/{namespace}/repos` | Create a repository; the namespace may be created implicitly |
| `GET` | `/namespaces/{namespace}/repos` | List repositories with search, sort, direction, cursor, and limit |
| `GET` | `/namespaces/{namespace}/repos/{repo}` | Read one repository and its Git remote |
| `PATCH` | `/namespaces/{namespace}/repos/{repo}/settings` | Change description, default branch, or read-only state |
| `GET` | `/namespaces/{namespace}/repos/{repo}/jobs` | List durable fork, import, and delete jobs while the repository is reachable |
| `DELETE` | `/namespaces/{namespace}/repos/{repo}` | Tombstone the repository and revoke its credentials |

## Observable proof

For a unique repository name owned by the current run:

1. `POST` create and require a successful Cloudflare-style envelope containing
   a non-empty ID, the requested name, a default branch, a tenant-qualified
   remote, and an initial credential. Treat the credential only as a secret.
2. `GET` the repository and require the same ID, account-qualified remote,
   default branch, description, read-only value, and `ready` behavior.
3. List its namespace and repositories and require the created names to appear.
4. When settings are in scope, patch one field at a time. Prove description
   readback, symbolic default-branch behavior, and that read-only blocks Git
   writes without blocking reads.
5. Delete the repository. Require HTTP `202`, then require repository REST
   lookup and Git discovery to stop reaching it. A credential issued before
   deletion must no longer authenticate.
6. A repeated delete is permitted to return the same repository ID; cleanup
   should be safe to retry.

## Gotchas

- Repository creation returns HTTP `200`, not `201`.
- The default branch is `main` when omitted. Changing the setting updates
  symbolic `HEAD`; it does not create a missing branch commit.
- Namespace jurisdiction is metadata only in this explicitly single-node
  implementation; do not claim physical data residency.
- Namespace and repository lists use cursor pagination. Credential lists use a
  different page/per-page scheme.
- Delete runs its durable tombstone transaction before returning `202`. It
  moves through `deleting` to `deleted`, revokes every repository credential,
  and hides the repository from normal REST and Git lookup.
- The repository-scoped jobs route cannot be used after deletion because normal
  repository lookup already hides deleting and deleted repositories.
- Immutable objects and lineage metadata are retained after deletion so a
  descendant snapshot can remain reconstructible. Cleanup must never delete
  shared object-storage data directly.

## Source anchors

- Route surface: `internal/api/server.go:52-80`
- Create and implicit namespace behavior: `internal/service/repo.go:10-55`
- Settings and symbolic `HEAD`: `internal/service/repo.go:128-163`
- Deleted repository lookup behavior: `internal/service/repo.go:175-193`
- Tombstone and credential revocation: `internal/jobs/delete.go:17-70`
- Repository result fields: `internal/types/repo.go:7-27`
- Name rules: `internal/types/name.go:9-65`
- Deletion and lineage contract: `docs/storage.md:29-33`
