# MIT licensing

Implemented 2026-10-25 from `7329c3227d468bbcedcbd879bbe6f12180993fda`, matching GitHub's `main` when checked that day. This working review replaces the uncommitted Apache 2.0 proposal after the user selected MIT. The policy is recorded in D17 in [AGENTS.md](../AGENTS.md).

## Scope

- [LICENSE](../LICENSE) contains the standard MIT text with `Copyright (c) 2026 Mike Rudolph`. The repository history lists Mike Rudolph as its sole author and begins in 2026. The user authorized the MIT grant.
- [README.md](../README.md) applies that license to original code, documentation, examples, and visual assets, with third-party exceptions.
- The documentation package and its lockfile identify MIT. Its build copies the root license and the pinned font's upstream OFL text into the published site, and the footer links to both.
- The production image identifies MIT in its OCI license label and includes the root license at `/usr/share/licenses/artifacts/LICENSE`. The Docker build context explicitly includes that source file.
- The MinIO test fixture keeps its existing upstream license copies. Other dependencies retain their own terms. This change does not establish a complete inventory or remediation of pre-existing third-party redistribution notices.

No source comment headers, public API changes, storage changes, CLA requirement, or runtime behavior changes are introduced. MIT permits commercial forks and competing hosted offerings. No Apache-specific NOTICE file is needed for the project's MIT grant.

## Verification

- Compared the project license with the [standard MIT text](https://opensource.org/license/mit) and checked package/lockfile consistency.
- `npm ci` installed the pinned documentation dependencies in this worktree. `npm run check` passed: 25 HTML pages, 942 internal links, 17 local assets, four SVG explainers, and all three diagram test files.
- Confirmed both published legal files exactly match their source files and that both links appear on all 21 pages with the documentation footer. `npm run predev` also passed.
- `make verify` passed formatting/imports, vet, and lint with zero lint issues, then stopped at `test-deps` because access to `/var/run/docker.sock` was denied. Race tests and coverage did not run.
- Ran `make complexity` separately; it passed. `go build -o /tmp/artifacts-mit-service ./cmd/artifacts` also passed.
- The task-owned production image build was blocked by the same Docker socket permissions. Verified the Docker source declarations and build-context allowlist, but built-image contents, label, and runtime readability remain unverified. No image inspection containers were created.
- Local Markdown links and whitespace checks passed. Confirmed the existing D1–D16 register text is unchanged.

Live REST/Git verification is not required for licensing and packaging alone. Remote CI must be checked for the pushed revision; the local results above do not establish CI status. A complete third-party distribution audit remains separate from adopting the project's MIT license.
