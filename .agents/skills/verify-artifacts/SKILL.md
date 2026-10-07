---
name: verify-artifacts
description: Validate a running Artifacts instance through real REST and Git, including binary commits, database schema isolation, IAM TLS and renewal, S3 encryption, and durable retries. Preserve redacted evidence and distinguish product failures from target, authentication, and evidence failures. Use for end-to-end change verification; do not use as a substitute for make verify.
---

# Verify Artifacts

Use the repository harness to prove observable behavior against a running Artifacts instance. The harness owns its uniquely named repository and client scratch state; it does not own or stop the target service, Postgres, or object storage.

## Route the work

- For the feature contract and expected surfaces, start with [references/features/README.md](references/features/README.md), then read only the feature references relevant to the request.
- Before interpreting, copying, or comparing a completed run, read [references/evidence-schema.md](references/evidence-schema.md).
- For database authentication or migration changes, read [database deployment](references/features/database-deployment.md). Local PostgreSQL and a signed-token test do not establish successful AWS authentication.
- For S3 encryption changes, read [encrypted object storage](references/features/s3-encryption.md). Local encryption and request-header checks do not establish AWS KMS permissions.

## Prepare and launch

If no target is already running, follow the repository README's local startup instructions in a separate terminal. Use a dedicated database and dedicated object/cache directories, bind to loopback, and record the processes or containers started by this task. Never stop or remove an unowned process, container, volume, or directory.

Set target configuration in the environment:

```text
ARTIFACTS_URL
ARTIFACTS_ACCOUNT
ARTIFACTS_API_TOKEN
```

Keep control and repository credentials in environment variables. Never print them, put them in a report, or pass them in Git command arguments. Treat a shared or production target as out of scope for the mutating drive unless the user explicitly authorizes it.

## Doctor

Run the read-only preflight first:

```bash
go run ./examples/agent-harness doctor
```

Proceed only when it reports a healthy instance. Preserve its distinction between an unreachable target, rejected authentication, a malformed response, and a healthy target; do not relabel those outcomes as product regressions.

## Drive the core workflow

Choose a new evidence directory outside client scratch state, then run:

```bash
go run ./examples/agent-harness verify-core --evidence DIR
```

The drive must prove the complete public workflow: REST repository creation, REST commit and readback, real Git clone and push, REST readback of Git-authored content, refs and WAL visibility, and repository deletion. Do not replace these checks with direct package calls or mock observations.

The command uses a unique repository name and attempts cleanup after both success and failure. Do not manually delete neighboring repositories when investigating a cleanup failure.

## Preserve and report evidence

For multipart uploads or large binary changes, run the binary drive after doctor; it includes the core workflow and adds byte-count/SHA-256 checks, executable mode, Git binary edits, and keyed retries after branch advancement:

```bash
go run ./examples/agent-harness verify-binary --evidence DIR --size-mib 100
```

Read [binary commits](references/features/binary-commits.md) for fixture sizing, capacity evidence, recovery checks, and interpretation. Use a fresh evidence directory for each run. The size is aggregate raw file content; use 512 only against a target configured to accept it with adequate disk capacity.

Keep `DIR` outside any temporary clone or disposable service directory. Confirm that `report.json`, `report.md`, and `git.log` remain after cleanup. Reports and logs must contain no control token, repository credential, credential-bearing URL, or authorization header.

Lead the result with the harness classification and the first failed assertion or step. Include the evidence directory and reproduction command. Do not claim success unless all assertions passed and cleanup confirmed the unique repository's final absence. Run `make verify` separately when the assignment also requires repository checks.

## Cleanup

Allow the harness cleanup path to remove only its unique repository and client-side scratch state. After the drive, stop only services launched and recorded by this task. Preserve the evidence directory even when verification or cleanup fails.
