# Multi-instance evidence

The [review](../../multi-instance.md) explains the implementation, exercised workload, and limits. These files describe one working-tree run, not future revisions.

Reproduce from the repository root with Docker, Go, Git, and Python available:

```sh
python3 reviews/evidence/multi-instance/reproduce.py
```

The script provisions uniquely named Postgres and MinIO containers, two authenticated server processes with separate caches, and a round-robin proxy. Credentials are generated in memory and passed through environment variables. It runs the verify-artifacts preflight and core workflow, then direct cross-instance checks. It removes only its owned processes, containers, and scratch files. Rerunning replaces this directory's generated evidence.

- `doctor.txt`: read-only preflight.
- `core/report.json`, `core/report.md`, `core/git.log`: complete REST → Git → REST evidence and repository cleanup.
- `checks.json`: additional cross-instance assertions and infrastructure cleanup.
- `routing.log`: proxy request placement, with no headers.
- `source.json`: source fingerprint for the tested implementation.

`checks.json` must have `passed: true`, every check must pass, and the core classification must be `none`. The reproduction checks the core Git log's recorded size and SHA-256 and scans evidence for generated secrets.
