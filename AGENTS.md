# Agent contract

Read this file and the assigned `tickets/*.md` before writing any code.

## Turn protocol

1. Implement **only** the assigned ticket.
2. Touch **only** `Allowed-paths`. Do not edit neighbors, rename exports, or start the next ticket.
3. If you need a new interface, new dependency, or to exceed a complexity limit: stop and report `BLOCKED`. Do not improvise.
4. Ship tests in the same turn. Test behavior and error paths, not mock call counts.
5. Run `make verify` before finishing. Changed packages must stay at ≥ 85% coverage.
6. Stop when the ticket `Done` criteria are met.

## Complexity budget

| Rule | Limit |
|---|---|
| File | 400 lines |
| Function | 70 lines |
| Cyclomatic | 15 |
| Nesting | 4 |
| Params | 6 |

`make complexity` and golangci-lint enforce this. Fail closed.

## Code rules

- Godoc on exported identifiers only. No narrative comments.
- No `util.go` / `helpers.go` dumping grounds.
- Prefer the standard library.
- Third-party deps only if listed on the ticket.
- Do not add abstractions ahead of need.
- Do not change frozen contracts in `internal/types`, `internal/store/object.Store`, or `internal/store/meta` interfaces.

## Tests

- Table-driven unit tests for parsers, names, envelope, CAS, and token expiry.
- `internal/testkit` for Postgres, MinIO, and real `git` after T5 exists.
- Every Cloudflare error code needs an error-path test once that surface exists.
- Forbidden: tautological tests; tests that only assert a mock was called.

## Verify

```
make verify
```

That is fmt, vet, lint, test, coverage, and file-length. CI runs the same target.

## Swarm

Do not spawn or start a ticket whose `Depends-on` is unfinished.
Two tickets must never edit the same file. If they would, the split is wrong — report `BLOCKED`.
