---
title: Local development
description: Run a repeatable local loop and collect evidence from the real REST and Git surfaces.
---

## Run the service

Use the [quickstart](/artifacts/getting-started/) for the complete first run. The short version, from the repository root:

```bash
docker compose up -d --wait postgres
export DATABASE_URL='postgres://artifacts:artifacts@localhost:5432/artifacts?sslmode=disable'
export ARTIFACTS_STORAGE=fs
export ARTIFACTS_DATA_DIR=./data
export ARTIFACTS_CACHE_DIR=./cache
go run ./cmd/artifacts dev
```

Keep the server in its own terminal. Use `dev --addr 127.0.0.1:8081` if you need another port. Development mode sets the returned Git remote origin to that listener. Use the [authenticated setup](/artifacts/core/authentication/) when testing credential scope, revocation, or expiry.

When running isolated experiments, use a dedicated database and dedicated object/cache directories. Track the processes and containers you start so cleanup affects only your experiment.

## Check the target before writing

In another terminal from the repository root:

```bash
export ARTIFACTS_URL=http://127.0.0.1:8080
export ARTIFACTS_ACCOUNT=local
# For serve mode, also supply ARTIFACTS_API_TOKEN in the environment.
go run ./examples/agent-harness doctor
```

`doctor` is read-only. It distinguishes an unreachable instance, rejected authentication, malformed responses, and a healthy target. A failed connection is not proof of a storage regression. Resolve preflight failures before running the mutating verification flow.

## Verify the public workflow

Against a disposable local instance you own:

```bash
EVIDENCE_DIR=$(mktemp -d)
go run ./examples/agent-harness verify-core --evidence "$EVIDENCE_DIR"
```

The harness creates a uniquely named repository and verifies REST creation, REST publication and exact readback, real Git clone/push, REST readback of Git-authored content, refs/WAL visibility, and repository deletion. It attempts cleanup after success or failure and only owns its test repository and client scratch state.

The evidence directory retains:

| File | Purpose |
| --- | --- |
| `report.json` | Machine-readable classification, steps, assertions, and artifact digests. |
| `report.md` | Human-readable report. |
| `git.log` | Sanitized real Git output. |

A passing run needs every assertion and final cleanup to succeed. Verify evidence remains after cleanup and contains no control token, repository credential, authorization header, or credential-bearing URL. Authentication boundaries must be tested with `serve`; passing on `dev` does not prove them.

## Check source changes

```bash
make verify
```

This runs formatting/import checks, vet, lint, race-enabled tests, coverage thresholds, and file-size checks. The formatter can update source files. Integration tests use Docker-backed dependencies, so make sure Docker is available.

The public workflow harness complements these checks. It does not replace them. For changes confined to the documentation site, build the site and inspect the generated links, search, and responsive layouts.

## Develop these docs

Content lives in the repository's `docs/` directory. Astro configuration, navigation, and presentation live in `docs-site/`.

```bash
cd docs-site
npm ci
npm run dev
```

Open `http://127.0.0.1:4321/artifacts/`. To check production search and output:

```bash
npm run build
npm run preview
```

Pagefind search is generated during the production build; the development server shows a search notice instead. See the [site README](https://github.com/mikerudolph/artifacts/blob/main/docs-site/README.md) for content and design conventions.
