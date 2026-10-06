import { createReadStream } from 'node:fs';
import { stat } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';

export async function fromFile(path) {
  const info = await stat(path);
  if (!info.isFile()) throw new TypeError('Expected a regular file');
  return { size: info.size, open: () => createReadStream(path) };
}

export function fromStream(open, size) {
  return { open, size };
}

function source(content) {
  if (typeof content === 'string') content = Buffer.from(content);
  if (content instanceof Uint8Array) {
    return { size: content.byteLength, open: () => [content] };
  }
  if (content instanceof Blob) {
    return { size: content.size, open: () => content.stream() };
  }
  if (typeof content?.open === 'function') return content;
  throw new TypeError('Content must be text, bytes, a Blob, or a reopenable stream');
}

export class CommitError extends Error {
  constructor(response, body) {
    super(body.errors?.[0]?.message ?? `Commit failed (${response.status})`);
    this.status = response.status;
    this.kind = body.errors?.[0]?.kind;
    this.currentHead = body.errors?.[0]?.current_head;
    this.source = body.errors?.[0]?.source;
    this.retryAfter = response.headers.get('retry-after');
  }
}

export async function commitFiles(repoURL, options) {
  const { token, idempotencyKey, expectedHead, branch, message, author, base,
    replace, deletes, files = [], signal, onProgress } = options;
  if (!idempotencyKey) throw new TypeError('Retain an idempotencyKey for this operation');
  const sources = files.map(file => source(file.content));
  const manifest = { branch, message, author, base, replace, deletes,
    expected_head: expectedHead, files: files.map((file, index) => ({
      path: file.path, part: `f${index}`, mode: file.mode, size: sources[index].size,
    })) };
  const boundary = `artifacts-${randomUUID()}`;
  const total = sources.every(item => item.size !== undefined)
    ? sources.reduce((sum, item) => sum + item.size, 0) : undefined;
  let bytes = 0;
  async function* body() {
    yield `--${boundary}\r\nContent-Disposition: form-data; name="manifest"\r\nContent-Type: application/json\r\n\r\n${JSON.stringify(manifest)}\r\n`;
    for (let index = 0; index < sources.length; index++) {
      yield `--${boundary}\r\nContent-Disposition: form-data; name="f${index}"; filename="content"\r\nContent-Type: application/octet-stream\r\n\r\n`;
      for await (const chunk of await sources[index].open()) {
        signal?.throwIfAborted();
        const data = typeof chunk === 'string' ? Buffer.from(chunk) : chunk;
        if (!(data instanceof Uint8Array)) throw new TypeError('Stream must yield bytes');
        bytes += data.byteLength;
        yield data;
        onProgress?.({ phase: 'uploading', bytes, total });
      }
      yield '\r\n';
    }
    yield `--${boundary}--\r\n`;
    onProgress?.({ phase: 'waiting', bytes, total });
  }
  const response = await fetch(`${repoURL.replace(/\/$/, '')}/commits`, {
    method: 'POST', body: body(), duplex: 'half', signal,
    headers: { Authorization: `Bearer ${token}`, 'Idempotency-Key': idempotencyKey,
      'Content-Type': `multipart/form-data; boundary=${boundary}` },
  });
  const result = await response.json();
  if (response.status !== 201 || !result.success) throw new CommitError(response, result);
  onProgress?.({ phase: 'complete', bytes, total });
  return result.result;
}
