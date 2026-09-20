# Storage and publication improvements: live verification

The core harness reported classification `none`; all core assertions and all 30 additional live checks passed. Repository cleanup was confirmed. The owned loopback server, isolated Docker Postgres container, and temporary filesystem storage were removed after the run.

Reproduce from the repository root with Docker and Go available:

```sh
python3 reviews/evidence/storage-improvements/reproduce.py
```

The script generates credentials in memory, performs the skill's doctor preflight and core REST → Git → REST workflow, and saves redacted evidence. It also verifies cold reads without a full local Git cache, incremental publication sizes, credential access, durable event replay, atomic multi-ref events, ref-only deletion, manual compaction, and background compaction after a process restart. Evidence is checked for credential leakage before completion.

- `core/report.json`, `core/report.md`, and `core/git.log`: standard harness evidence and sanitized Git output.
- `adoption-checks.json`: all additional assertions.
- `publication-costs.json`: actual pack sizes for this fixture, excluding index files and transport overhead. These are a correctness/cost regression fixture, not a general performance benchmark.
- `doctor.txt`: authenticated preflight result.
