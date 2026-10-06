# Binary commits

These examples publish files in one multipart REST commit. They require the multipart implementation in this repository. The Node helper and Python example stream file contents without collecting the complete upload in memory. They are application examples, not a supported SDK.

With Node 22 or later, import the local helper:

```js
import { commitFiles, fromFile } from './commit.mjs';

const result = await commitFiles(process.env.REPO_API, {
  token: process.env.ARTIFACTS_TOKEN,
  idempotencyKey: process.env.OPERATION_ID,
  expectedHead: process.env.EXPECTED_HEAD,
  message: 'Publish report',
  files: [
    { path: 'report.md', content: '# Report\n' },
    { path: 'report.pdf', content: await fromFile('./report.pdf') },
    { path: 'run.sh', content: 'echo ready\n', mode: '100755' },
  ],
  signal: AbortSignal.timeout(20 * 60 * 1000),
});
console.log(result.sha);
```

`fromStream(() => stream, optionalSize)` accepts a factory yielding a fresh byte stream on each call. An HTTP response body is usable once; automatic retries require a factory that can reproduce identical content. The helper makes one attempt. Retry transient failures with a bounded backoff and the same operation ID, unchanged metadata, and unchanged content; honor `Retry-After`. A head conflict needs application reconciliation and a new operation ID. Changing a file on disk between attempts changes the operation and can produce an idempotency conflict. Keep the original inputs available until the outcome is known.

`onProgress` receives `uploading`, `waiting`, and `complete` phases. Byte counts measure content pulled from the sources, not server durability or network acknowledgements. `waiting` means the request body was produced and the client awaits the result. Only `complete` follows a successful publication response. `CommitError` exposes `status`, `kind`, `currentHead`, `source`, and `retryAfter`.

Python 3.10 or later can use `upload.py` without dependencies. Set `REPO_API`, `ARTIFACTS_TOKEN`, `OPERATION_ID`, `EXPECTED_HEAD`, and `UPLOAD_FILE` in the environment, then run:

```sh
python3 examples/binary-commits/upload.py
```

For curl, save this manifest as `manifest.json`. Omitted sizes allow streams of unknown length; the server still counts and limits every byte.

```json
{"expected_head":"","files":[{"path":"report.pdf","part":"report"}]}
```

Use `expected_head: ""` only for an unborn branch; otherwise use the SHA you observed. With the credential supplied through the environment:

```sh
curl --config - --fail-with-body \
  -H "Idempotency-Key: $OPERATION_ID" \
  -F 'manifest=@manifest.json;type=application/json' \
  -F 'report=@report.pdf;type=application/octet-stream' \
  "$REPO_API/commits" <<EOF
header = "Authorization: Bearer $ARTIFACTS_TOKEN"
EOF
```

One request produces one commit. All retry attempts resend the complete body, including after a lost success response. Read files at the returned SHA to keep related reads consistent. See [writing files](../../docs/core/writing-files.md) for limits, conflicts, and recovery.
