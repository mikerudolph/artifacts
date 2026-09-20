**Developer interface verification**

The core harness reported classification `none`, with all assertions passing and the owned repository absent after cleanup. See [the core report](core/report.md) and [additional interface checks](interface-checks.json).

The additional checks use authenticated `serve` mode and real HTTP/Git clients. They cover incremental edits across REST and Git, detection of an intervening Git push, replay after stopping and restarting the server process, unchanged publication counts on replay, credential scope, and revocation on both transports.

From the repository root, reproduce against a new disposable Postgres container and an isolated loopback server:

```bash
python3 reviews/evidence/interface-changes/reproduce.py
```

Requires Docker, Go, Git, and Python 3. The script starts and stops only its own container and server, removes its temporary data/cache/clone, and leaves the reports here. Credentials are generated for the run and passed through process environments; reports are checked for secret leakage. Running again replaces these evidence files.

The script invokes the repository's read-only `doctor` and `verify-core` harness before the additional checks. It validates the recorded Git log digest and size. It does not replace `make verify`, which separately runs formatting, vet, lint, race tests, coverage thresholds, and complexity checks.

The code-level regressions also cover independent server handlers sharing Postgres, transaction rollback, explicit deletion and empty trees, branching from a selected commit, preservation of binary content/executable modes/symlinks, path collisions, malformed requests, and read-only enforcement at publication.
