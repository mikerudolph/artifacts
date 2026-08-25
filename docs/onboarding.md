# Agent onboarding

Use one repository per agent session or unit of work. Accounts are tenants, namespaces group a team or environment, and repositories isolate history and credentials.

## Recommended flow

1. The orchestrator creates `agents/<agent>-<session>-work` over REST.
2. It publishes bootstrap instructions and inputs with `POST .../repos/<repo>/commits`.
3. It reads a bootstrap file with the REST file endpoint.
4. It issues a short-lived credential with `POST .../credentials`.
5. The agent clones the tenant-qualified remote, commits outputs, and pushes.
6. The orchestrator reads results over REST or creates a metadata-only snapshot fork for the next session.
7. It deletes the repository when the session is over. Delete blocks access immediately; immutable data still reachable by snapshot forks is retained.

REST base:

```text
/client/v4/accounts/{account}/artifacts
```

Important routes:

| Method | Route | Purpose |
|---|---|---|
| POST | `/namespaces/{ns}/repos` | Create repo, symbolic `HEAD`, and initial write credential |
| POST | `/namespaces/{ns}/repos/{repo}/commits` | Publish up to 100 files / 1 MiB through the pack WAL |
| GET | `/namespaces/{ns}/repos/{repo}/file?ref=main&path=x` | Read a file |
| POST | `/namespaces/{ns}/credentials` | Issue repository credential (`repo`, `scope`, `ttl`) |
| GET | `/namespaces/{ns}/repos/{repo}/refs` | Inspect published refs |
| GET | `/namespaces/{ns}/repos/{repo}/wal` | Inspect immutable publications |
| PATCH | `/namespaces/{ns}/repos/{repo}/settings` | Update description, default branch, or read-only state |
| POST | `/namespaces/{ns}/repos/{repo}/fork` | Create a same-tenant snapshot fork |
| POST | `/namespaces/{ns}/repos/{repo}/import` | Import a public HTTPS Git remote |

Git remote:

```text
{PUBLIC_URL}/git/{account}/{namespace}/{repo}.git
```

Repository credentials accept Bearer auth. Basic auth is also supported with the credential (or its secret before `?expires=`) as the password. Read credentials cannot push. Revoked, expired, wrong-repository, and cross-tenant credentials are rejected.

Imports reject non-HTTPS URLs, userinfo, localhost, and private, loopback, unspecified, or link-local IP destinations. Keep import workers behind normal outbound network controls as defense in depth.

Run the complete executable example:

```bash
ARTIFACTS_URL=http://127.0.0.1:8080 ARTIFACTS_ACCOUNT=local \
  go run ./examples/agent-harness
```
