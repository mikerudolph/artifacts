# Verification evidence

Read this reference when reviewing, transporting, or comparing a `verify-core` run.

## Files

The evidence directory survives client and repository cleanup and contains:

- `report.json`: machine-readable source of truth.
- `report.md`: concise human-readable rendering of the same run.
- `git.log`: sanitized output from real Git operations.

Treat missing, unreadable, internally inconsistent, or credential-bearing evidence as an evidence failure, even if the drive otherwise appeared successful.

## `report.json`

The report contains:

| Field | Meaning |
|---|---|
| `run_id` | Unique identifier shared by the drive and its owned repository. |
| `started_at`, `finished_at` | Run boundaries used to correlate steps and logs. |
| `configuration` | Target URL and account plus a redacted API-token value. |
| `classification` | One of `none`, `unreachable`, `authentication`, `unexpected_response`, `assertion`, `product`, `cleanup`, or `evidence`. |
| `failure` | Sanitized failure detail; empty on success. |
| `steps[]` | Ordered `name`, `success`, `duration_ms`, and sanitized `detail` values. |
| `assertions[]` | Behavioral `name`, `passed`, and sanitized `detail` values. |
| `artifacts[]` | Evidence-relative `path`, SHA-256 `sha256`, and byte `size`. |

Artifact paths must remain inside the evidence directory. `report.json` and `report.md` are not self-listed because their digests would be recursive. Recompute each listed artifact digest over its exact bytes and compare it with the lowercase hexadecimal SHA-256 and byte size before relying on copied or archived evidence. A digest or size mismatch invalidates the evidence rather than proving a product regression.

## Required proof

A passing core report must show successful assertions for:

- REST create, commit, and file readback.
- Real Git clone and push without credentials in arguments or logs.
- REST readback of Git-authored content.
- Published refs and WAL visibility.
- Final absence of the uniquely named repository after cleanup.
- Continued presence of the evidence files after cleanup.

Cleanup is part of the result. A functional drive followed by an unremoved repository is not a pass.

## Redaction

The following must never appear in `report.json`, `report.md`, `git.log`, filenames, or artifact paths:

- `ARTIFACTS_API_TOKEN` contents.
- Repository credential plaintext, including the secret before an `?expires=` suffix.
- Authorization header values.
- Git remote URLs containing userinfo.

Configuration records `api_token` as `<redacted>` when configured and `<unset>` otherwise. Target URLs must omit userinfo and replace the value of any query key containing `token`, `key`, `secret`, or `password` with `<redacted>`. When reporting a failure, sanitize response bodies and command output before persisting them.

## Interpreting failure

Use `classification` before diagnosing details. `none` means no classified failure. Preserve every other harness category and cite the first failed step or assertion. Distinguish target reachability, authentication, unexpected responses, assertion failures, product behavior, and cleanup failures; do not collapse them into a generic test failure.

When evidence is insufficient or contradictory, report that limitation and reproduce the run. Do not infer success from an exit code, the agent's narrative, or Git output alone.
