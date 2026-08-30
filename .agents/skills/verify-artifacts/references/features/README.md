# Artifacts feature map

These references describe behavior implemented by the current repository. Load
only the document needed for the verification being performed.

| Feature | Reference | Use when |
|---|---|---|
| Repository lifecycle | [repository-lifecycle.md](repository-lifecycle.md) | Creating, finding, configuring, or deleting a tenant-scoped repository |
| REST/Git interoperability | [rest-git-interoperability.md](rest-git-interoperability.md) | Proving content published through one interface is visible through the other |
| Credentials | [credentials.md](credentials.md) | Authenticating REST or Git and checking repository credential boundaries |

The core drive crosses all three features:

1. Create a uniquely named repository over REST.
2. Publish and read bootstrap content over REST.
3. Clone with the returned repository credential, commit, and push with real
   Git.
4. Read the Git-authored content over REST and inspect refs and WAL.
5. Delete the unique repository and prove it is no longer reachable.

Verification must assert final behavior, preserve redacted evidence, and clean
up only the uniquely named repository and client-side scratch state it owns.
Never place control-plane or repository credentials in reports, logs, or Git
command arguments.

## Source anchors

- Mounted REST routes: `internal/api/server.go:48-80`
- Combined REST, Git, and development browser wiring: `internal/app/app.go:34-87`
- Executable REST to Git to REST drive: `examples/agent-harness/drive.go:52-159`
- Recommended agent/session workflow: `docs/onboarding.md:5-13`
