---
title: Web UI
description: Browse and inspect the repositories on your local development instance.
---

The web UI is available at [http://127.0.0.1:8080](http://127.0.0.1:8080) while `go run ./cmd/artifacts dev` is running. Complete the [quickstart](/artifacts/getting-started/) through the Git readback step, before deleting its repository, to have content to inspect.

## Find a repository

Open **local tenant**, then choose a namespace and repository. For the quickstart, select `research` and the `quickstart-...` repository you created. The URL follows the hierarchy:

```text
http://127.0.0.1:8080/local/research/{repository-name}
```

An empty namespace or repository view can be normal before you create and seed data. Creating a repository establishes its identity, but you must publish a commit before it contains a readable file tree.

## Read the Code view

Choose a branch and click **View**. The page shows the resolved commit, directory entries, and file modes. Open a directory to browse its children; the parent link goes up a level while preserving the selected ref.

Open `outputs/report.md` to preview the published report. The UI renders bounded text content as escaped text, not executable HTML. Valid UTF-8 text, JSON, and XML may be previewed up to **256 KiB**. Binary, invalid UTF-8, or larger content is offered as a download.

That preview limit is a UI limit. REST can stream larger files. The browser never downloads WAL packs or constructs repositories itself; the server handles materialization.

## Inspect the repository tabs

| Tab | What it shows | When it helps |
| --- | --- | --- |
| Code | Branch selection, directories, file previews, and downloads. | Confirm the worker published the expected files at the right path. |
| Commits | Commit messages, authors, dates, and abbreviated SHAs. | Follow a session's published history. |
| WAL | Publication sequences, pack checksums, sizes, and timestamps. | Diagnose storage publication; these are not user files. |
| Settings | Default branch, read-only state, and WAL sequence. | Inspect effective repository settings. |

The Settings tab is read-only. Change settings through the REST API. The UI does not create repositories, edit or upload files, fork repositories, or manage credentials.

## Investigate a surprising result

If a file looks old, check the branch and resolved commit first. A worker's local commit does not appear until it pushes. If inputs disappeared after a REST update, check whether the request included the whole intended snapshot. The [write guide](/artifacts/core/writing-files/) explains that behavior.

For empty or error views, try the corresponding REST `/tree` or `/file` request and inspect its HTTP status. A cold read may need to reconstruct the cache. WAL entries confirm publications but do not prove the meaning or correctness of a worker's output.

## Development scope

The UI is mounted only by `artifacts dev`, which disables authentication and binds to loopback. `serve` exposes authenticated REST and Git without this UI. Do not use the local browser as a test of production authentication or expose the development server publicly.

For repeatable verification beyond visual inspection, use the [public workflow harness](/artifacts/developer-tools/local-development/#verify-the-public-workflow).
