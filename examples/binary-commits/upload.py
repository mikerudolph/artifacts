import http.client
import json
import os
import pathlib
import uuid
from urllib.parse import urlsplit


def commit_files(repo_url, token, operation_id, expected_head, files):
    boundary = "artifacts-" + uuid.uuid4().hex
    sources = [(destination, pathlib.Path(source)) for destination, source in files]
    manifest = {
        "expected_head": expected_head,
        "message": "Publish files",
        "files": [
            {"path": destination, "part": f"f{index}", "size": source.stat().st_size}
            for index, (destination, source) in enumerate(sources)
        ],
    }

    def body():
        yield (f'--{boundary}\r\nContent-Disposition: form-data; name="manifest"\r\n'
               'Content-Type: application/json\r\n\r\n').encode()
        yield json.dumps(manifest).encode()
        yield b"\r\n"
        for index, (_, source) in enumerate(sources):
            yield (f'--{boundary}\r\nContent-Disposition: form-data; name="f{index}"; '
                   'filename="content"\r\nContent-Type: application/octet-stream\r\n\r\n').encode()
            with source.open("rb") as stream:
                while chunk := stream.read(64 * 1024):
                    yield chunk
            yield b"\r\n"
        yield f"--{boundary}--\r\n".encode()

    target = urlsplit(repo_url)
    if target.scheme not in ("http", "https") or target.username or target.password:
        raise ValueError("Expected an HTTP repository URL without credentials")
    connection_type = http.client.HTTPSConnection if target.scheme == "https" else http.client.HTTPConnection
    connection = connection_type(target.hostname, target.port, timeout=1200)
    try:
        connection.request("POST", target.path.rstrip("/") + "/commits", body=body(),
                           headers={"Authorization": "Bearer " + token,
                                    "Idempotency-Key": operation_id,
                                    "Content-Type": "multipart/form-data; boundary=" + boundary},
                           encode_chunked=True)
        response = connection.getresponse()
        result = json.loads(response.read(65536))
        if response.status != 201 or not result.get("success"):
            error = result.get("errors", [{}])[0]
            raise RuntimeError(f"HTTP {response.status}: {error.get('kind')} {error.get('message')}")
        return result["result"]
    finally:
        connection.close()


if __name__ == "__main__":
    result = commit_files(os.environ["REPO_API"], os.environ["ARTIFACTS_TOKEN"],
                          os.environ["OPERATION_ID"], os.environ["EXPECTED_HEAD"],
                          [("report.pdf", os.environ["UPLOAD_FILE"])])
    print(json.dumps(result))
