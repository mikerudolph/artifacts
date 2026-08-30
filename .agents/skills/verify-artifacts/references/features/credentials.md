# Credentials and authentication

## Scope

Artifacts has two distinct credential planes:

- A tenant-bound control-plane bearer token authorizes REST requests.
- A repository-bound credential authorizes Git smart HTTP reads or writes.

They are not interchangeable. Durable records contain hashes rather than
plaintext secrets.

## User paths

| Interface | Path or command | Behavior |
|---|---|---|
| CLI | `artifacts token create --account ACCOUNT` | Mint a tenant-bound control-plane token |
| REST | `POST /namespaces/{namespace}/credentials` | Mint a repository credential from `repo`, `scope`, and `ttl` |
| REST | `GET /namespaces/{namespace}/repos/{repo}/credentials` | List credential metadata by state and page |
| REST | `DELETE /namespaces/{namespace}/credentials/{id}` | Revoke a repository credential |
| Git | `Authorization: Bearer {credential}` | Authenticate clone/fetch/push |
| Git | HTTP Basic password | Authenticate with the credential; username is ignored |

The older `/tokens` REST paths are aliases of `/credentials`.

## Observable proof

Run credential checks against authenticated `serve` mode, never the no-auth
development server.

1. Send no control token and an invalid control token to REST; require HTTP
   `401`. Send a valid token for the route account and require access.
2. Use the initial write credential returned by repository creation to clone
   and push. Exercise both Bearer and Basic transport without placing the
   secret in a report or Git command argument.
3. Mint a read credential. Require clone/fetch to succeed and push discovery or
   receive to fail with HTTP `403`.
4. Revoke a credential and require subsequent Git access to return `401`.
5. Present active credentials to another repository and another tenant route;
   require `401` and no repository content disclosure.
6. For expiry checks, mint at the minimum supported TTL or use deterministic
   package tests; require the expired credential to return `401`.
7. List credentials and require IDs, scopes, state, creation, and expiry
   metadata while confirming plaintext is absent.

## Secret-handling rules

- Keep control and repository credentials in environment variables or protected
  process input only.
- Never include credentials in Git command arguments, remote URLs, reports,
  captured commands, or failure messages.
- Redact both full credentials and secret-only forms before writing evidence.
- The repository credential returned by create may include `?expires={unix}`.
  Treat the entire value as secret even when using its secret-only form.

## Gotchas

- `artifacts dev` bypasses both control-plane and repository authentication, so
  it cannot prove credential boundaries.
- REST accepts only control-plane Bearer credentials. Repository credentials
  work only on Git smart HTTP.
- Repository credentials have `read` or `write` scope. Empty scope defaults to
  `write`.
- TTL is expressed in seconds: zero defaults to 24 hours, with a valid range of
  60 seconds through one year.
- A full repository credential has the form
  `art_v1_{40 lowercase hex}?expires={unix}`. The prefix and expiry suffix may
  be omitted when authenticating, but stored expiry, state, repository binding,
  and requested write scope are always checked.
- Bearer parsing for Git is case-insensitive. Basic auth uses only the password.
- Credential list state defaults to `active`; accepted filters are `active`,
  `expired`, `revoked`, and `all`.
- Repository read-only state independently forbids Git writes even when the
  credential has write scope.

## Source anchors

- REST authentication middleware: `internal/api/auth.go:13-34`
- Tenant binding for control tokens: `internal/auth/issuer.go:56-75`
- Credential routes and aliases: `internal/api/server.go:61-66`
- Repository credential authorization: `internal/auth/repository.go:26-46`
- Bearer and Basic extraction: `internal/githttp/server.go:46-59`
- Git read/write enforcement: `internal/githttp/repository.go:64-86`
- Credential format and parsing: `internal/auth/auth.go:17-79`
- Scope and state rules: `internal/types/enums.go:3-61`
- TTL rules: `internal/types/ttl.go:3-20`
