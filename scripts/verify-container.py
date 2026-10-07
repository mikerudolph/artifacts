import argparse
import base64
import concurrent.futures
import hashlib
import json
import os
import pathlib
import re
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

root = pathlib.Path.cwd()
parser = argparse.ArgumentParser()
parser.add_argument("--image", required=True)
parser.add_argument("--evidence", required=True)
parser.add_argument("--binary-size-mib", type=int, action="append", default=[])
parser.add_argument("--database-schema", default="")
parser.add_argument("--s3-sse", choices=["AES256", "aws:kms"], default="")
args = parser.parse_args()
if args.database_schema and (not re.fullmatch(r"[a-z_][a-z0-9_]{0,62}", args.database_schema)
                             or args.database_schema.startswith("pg_") or args.database_schema == "information_schema"):
    parser.error("database schema must be a lowercase non-system identifier")
schema = args.database_schema or "public"
if any(size < 1 or size > 512 for size in args.binary_size_mib):
    parser.error("binary sizes must be between 1 and 512 MiB")
evidence = pathlib.Path(args.evidence).resolve()
evidence.mkdir(parents=True, exist_ok=True)
scratch = pathlib.Path(tempfile.mkdtemp(prefix="artifacts-container-"))
prefix = "artifacts-container-" + secrets.token_hex(5)
containers, checks, secret_values = [], [], []
volumes = []
network = prefix + "-network"
minio_image = "artifacts-test-minio:2025-09-07"
completed = False
servers = []
base = "/client/v4/accounts/local/artifacts"
token, password, access, secret = [secrets.token_hex(24) for _ in range(4)]
secret_values.extend([token, password, access, secret])
env = dict(os.environ, POSTGRES_PASSWORD=password, MINIO_ROOT_USER=access,
           MINIO_ROOT_PASSWORD=secret, AWS_ACCESS_KEY_ID=access, AWS_SECRET_ACCESS_KEY=secret,
           GIT_CONFIG_GLOBAL="/dev/null", GIT_CONFIG_NOSYSTEM="1", GIT_TERMINAL_PROMPT="0")
env.pop("AWS_SESSION_TOKEN", None)
env.pop("MINIO_KMS_SECRET_KEY", None)
if args.s3_sse:
    kms_secret = "artifacts-test:" + base64.b64encode(secrets.token_bytes(32)).decode()
    secret_values.extend([kms_secret, kms_secret.split(":", 1)[1]])
    env["MINIO_KMS_SECRET_KEY"] = kms_secret

def command(args, **kwargs):
    result = subprocess.run(args, capture_output=True, text=True, **kwargs)
    if result.returncode:
        detail = (result.stdout + "\n" + result.stderr).strip()
        for value in secret_values:
            detail = detail.replace(value, "<redacted>")
        raise RuntimeError("command failed: " + args[0] + "\n" + detail[-4096:])
    return result.stdout.strip()

def check(name, condition):
    checks.append({"name": name, "passed": bool(condition)})
    if not condition:
        raise AssertionError(name)

def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]

def request(node, method, path, credential=token, body=None, key=None):
    headers = {"Authorization": "Bearer " + credential}
    if body is not None:
        headers["Content-Type"] = "application/json"
    if key:
        headers["Idempotency-Key"] = key
    req = urllib.request.Request(origins[node] + base + path,
        data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()

def api(node, method, path, credential=token, body=None, key=None, want=200):
    status, data = request(node, method, path, credential, body, key)
    if status != want:
        raise AssertionError(f"{method} {path}: HTTP {status}, expected {want}")
    return json.loads(data)["result"]

def probe(node, path):
    try:
        with urllib.request.urlopen(origins[node] + path, timeout=5) as response:
            return response.status
    except urllib.error.HTTPError as error:
        return error.code

def launch(node):
    name = prefix + f"-server-{node}"
    node_env = dict(env, DATABASE_URL=runtime_dsn, ARTIFACTS_API_TOKEN="",
                    ARTIFACTS_SKIP_MIGRATIONS="true", ARTIFACTS_HTTP_ADDR=":8080")
    keys = ["DATABASE_URL", "ARTIFACTS_API_TOKEN", "ARTIFACTS_SKIP_MIGRATIONS",
            "ARTIFACTS_DATABASE_SCHEMA", "ARTIFACTS_DATABASE_AUTH",
            "ARTIFACTS_HTTP_ADDR", "ARTIFACTS_PUBLIC_URL", "ARTIFACTS_STORAGE", "S3_BUCKET",
            "S3_REGION", "S3_ENDPOINT", "S3_USE_PATH_STYLE", "S3_SSE", "S3_SSE_KMS_KEY_ID",
            "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"]
    cache = ["--tmpfs", "/var/cache/artifacts:rw,uid=10001,gid=10001,mode=0700"]
    if args.binary_size_mib:
        volume = prefix + "-cache-" + str(len(volumes))
        command(["docker", "volume", "create", volume])
        volumes.append(volume)
        cache = ["--mount", f"type=volume,src={volume},dst=/var/cache/artifacts"]
    command(["docker", "run", "-d", "--name", name, "--network", network, "--read-only",
             "--cap-drop=ALL", "--security-opt=no-new-privileges", *cache, "-p",
             "127.0.0.1:" + origins[node].rsplit(":",1)[1] + ":8080",
             *[part for key in keys for part in ["-e", key]], args.image], env=node_env)
    containers.append(name)
    for _ in range(200):
        try:
            if probe(node, "/readyz") == 200:
                info = json.loads(command(["docker", "inspect", name]))[0]
                check(f"node {node} non-root and read-only", info["Config"]["User"] == "10001:10001" and info["HostConfig"]["ReadonlyRootfs"])
                return name
        except OSError:
            pass
        time.sleep(.1)
    raise RuntimeError("owned server readiness timeout")

def stop(name):
    command(["docker", "stop", "--time", "40", name])
    check("SIGTERM exits cleanly", command(["docker", "inspect", "-f", "{{.State.ExitCode}}", name]) == "0")
    command(["docker", "rm", name])
    containers.remove(name)

def setup(*arguments, token_value=token, expected=0, database_dsn=None):
    setup_env = dict(env, ARTIFACTS_BOOTSTRAP_TOKEN=token_value)
    if database_dsn:
        setup_env["DATABASE_URL"] = database_dsn
    result = subprocess.run(["docker", "run", "--rm", "--read-only", "--network", network,
                             "-e", "DATABASE_URL", "-e", "ARTIFACTS_BOOTSTRAP_TOKEN",
                             "-e", "ARTIFACTS_DATABASE_SCHEMA", "-e", "ARTIFACTS_DATABASE_AUTH",
                             args.image, *arguments], env=setup_env, capture_output=True, text=True)
    check("setup command " + arguments[0], result.returncode == expected)
    check("setup output redacts credentials", not any(value in result.stdout + result.stderr for value in secret_values))

def git(*args, expect_success=True):
    result = subprocess.run(["git", "--config-env=http.extraHeader=ARTIFACTS_GIT_AUTH", *args],
                            env=git_env, capture_output=True, text=True)
    if expect_success and result.returncode:
        raise AssertionError("Git operation failed")
    return result

def database_sql(query):
    return command(["docker", "exec", "-i", name, "psql", "-U", "artifacts", "-d", "artifacts", "-At", "-v", "ON_ERROR_STOP=1"], input=query)

def provision_schema():
    if not args.database_schema:
        return
    owner_password = secrets.token_hex(24)
    secret_values.append(owner_password)
    database_sql(f"CREATE ROLE migration LOGIN PASSWORD '{owner_password}'; CREATE SCHEMA {schema} AUTHORIZATION migration; "
                 "CREATE TABLE public.accounts(sentinel text); INSERT INTO public.accounts VALUES ('untouched'); "
                 "CREATE TABLE public.schema_migrations(version bigint,dirty boolean); INSERT INTO public.schema_migrations VALUES (999,false);")
    env["DATABASE_URL"] = f"postgres://migration:{owner_password}@{name}/artifacts?sslmode=disable&pool_max_conns=4"
    check("migration role does not need database CREATE", database_sql("SELECT has_database_privilege('migration','artifacts','CREATE')") == "f")

def check_neighbor():
    if args.database_schema:
        state = database_sql("SELECT sentinel FROM public.accounts; SELECT version FROM public.schema_migrations; "
                             "SELECT count(*) FROM pg_tables WHERE schemaname='public';")
        check("neighboring application data and migration ledger unchanged", state == "untouched\n999\n2")

def verify_binary_clients():
    repo_name = "clients-" + secrets.token_hex(4)
    path = "/namespaces/binary-clients/repos/" + repo_name
    created = api(0, "POST", "/namespaces/binary-clients/repos", body={"name": repo_name})
    credential = created["credential"]["plaintext"]
    secret_values.extend([credential, credential.split("?")[0]])
    payload = scratch / "client-source.bin"
    payload.write_bytes(bytes(range(256)))
    client_env = dict(env, REPO_API=origins[0] + base + path, ARTIFACTS_TOKEN=credential,
                      OPERATION_ID="node-client", EXPECTED_HEAD="", UPLOAD_FILE=str(payload))
    node_source = """
import { commitFiles, fromFile, fromStream } from './examples/binary-commits/commit.mjs';
const result = await commitFiles(process.env.REPO_API, {
  token: process.env.ARTIFACTS_TOKEN,
  idempotencyKey: process.env.OPERATION_ID,
  expectedHead: process.env.EXPECTED_HEAD,
  files: [
    {path: 'node.bin', content: await fromFile(process.env.UPLOAD_FILE)},
    {path: 'stream.bin', content: fromStream(async function* () { yield new Uint8Array([0, 255]); })},
  ],
});
console.log(JSON.stringify(result));
"""
    first = json.loads(command(["node", "--input-type=module"], input=node_source, env=client_env))
    check("Node helper streams file bytes", request(1, "GET", path + "/file?path=node.bin", credential) == (200, bytes(range(256))))
    check("Node helper accepts unknown-size stream", request(1, "GET", path + "/file?path=stream.bin", credential) == (200, bytes([0, 255])))
    python_env = dict(client_env, OPERATION_ID="python-client", EXPECTED_HEAD=first["sha"])
    command(["python3", "examples/binary-commits/upload.py"], env=python_env)
    check("Python example streams file bytes", request(1, "GET", path + "/file?path=report.pdf", credential) == (200, bytes(range(256))))
    replay = json.loads(command(["node", "--input-type=module"], input=node_source, env=client_env))
    check("Node helper replays after Python publication", replay == first and len(api(0, "GET", path + "/wal")) == 2)
    api(0, "DELETE", path, want=202)
    check("client example repository cleaned up", request(1, "GET", path)[0] == 404)

try:
    command(["docker", "network", "create", network])
    command(["docker", "build", "-t", minio_image, str(root / "test/minio")])
    name = prefix + "-postgres"
    command(["docker", "run", "-d", "--rm", "--name", name, "--network", network, "-e", "POSTGRES_PASSWORD",
             "-e", "POSTGRES_USER=artifacts", "-e", "POSTGRES_DB=artifacts",
             "-p", "127.0.0.1::5432", "postgres:16-alpine"], env=env)
    containers.append(name)
    for _ in range(200):
        if subprocess.run(["docker", "exec", name, "pg_isready", "-U", "artifacts"],
                          capture_output=True).returncode == 0:
            break
        time.sleep(.1)
    storage = prefix + "-minio"
    command(["docker", "run", "-d", "--rm", "--name", storage, "--network", network,
             "-e", "MINIO_ROOT_USER", "-e", "MINIO_ROOT_PASSWORD", "-e", "MINIO_KMS_SECRET_KEY", "-p", "127.0.0.1::9000",
             minio_image,
             "server", "/data"], env=env)
    containers.append(storage)
    s3_port = command(["docker", "port", storage, "9000/tcp"]).rsplit(":", 1)[1]
    for _ in range(200):
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{s3_port}/minio/health/ready", timeout=2):
                break
        except OSError:
            time.sleep(.1)
    bucket_env = dict(env, MC_HOST_local=f"http://{access}:{secret}@127.0.0.1:9000")
    command(["docker", "exec", "-e", "MC_HOST_local", storage, "mc", "mb", "local/artifacts"], env=bucket_env)
    origins = [f"http://127.0.0.1:{free_port()}" for _ in range(2)]
    env.update(DATABASE_URL=f"postgres://artifacts:{password}@{name}/artifacts?sslmode=disable",
        ARTIFACTS_DATABASE_SCHEMA=args.database_schema, ARTIFACTS_DATABASE_AUTH="dsn",
        ARTIFACTS_PUBLIC_URL=origins[0], ARTIFACTS_URL=origins[0], ARTIFACTS_ACCOUNT="local",
        ARTIFACTS_DEFAULT_ACCOUNT="local", ARTIFACTS_AUTH="token", ARTIFACTS_API_TOKEN=token,
        ARTIFACTS_STORAGE="s3", S3_BUCKET="artifacts", S3_REGION="us-east-1",
        S3_ENDPOINT=f"http://{storage}:9000", S3_USE_PATH_STYLE="true", S3_PREFIX="",
        S3_SSE=args.s3_sse, S3_SSE_KMS_KEY_ID="artifacts-test" if args.s3_sse == "aws:kms" else "")
    provision_schema()
    setup("bootstrap", "--account", "local", expected=1)
    setup("migrate")
    setup("migrate")
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        list(pool.map(lambda _: setup("bootstrap", "--account", "local"), range(2)))
    setup("bootstrap", "--account", "other", expected=1)
    state = database_sql(f"SELECT count(*) FROM {schema}.api_tokens; SELECT count(*) FROM {schema}.accounts WHERE id='other';")
    check("bootstrap retries preserve one token and rejected account rolls back", state == "1\n0")
    runtime_password = secrets.token_hex(24)
    secret_values.append(runtime_password)
    runtime_dsn = f"postgres://runtime:{runtime_password}@{name}/artifacts?sslmode=disable"
    database_sql(f"CREATE ROLE runtime LOGIN PASSWORD '{runtime_password}'; GRANT CONNECT ON DATABASE artifacts TO runtime; GRANT USAGE ON SCHEMA {schema} TO runtime; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA {schema} TO runtime; REVOKE INSERT, UPDATE, DELETE ON {schema}.schema_migrations FROM runtime;")
    permission = database_sql(f"SELECT has_schema_privilege('runtime', '{schema}', 'CREATE');")
    check("runtime role cannot create schema objects", permission == "f")
    if args.database_schema:
        check("runtime cannot change neighboring application or migration history", database_sql(
            f"SELECT has_table_privilege('runtime','public.accounts','UPDATE'); SELECT has_table_privilege('runtime','{schema}.schema_migrations','UPDATE');") == "f\nf")
        setup("bootstrap", "--account", "local", database_dsn=runtime_dsn)
        check_neighbor()
    image_info = json.loads(command(["docker", "image", "inspect", args.image]))[0]
    (evidence / "image.json").write_text(json.dumps({"id":image_info["Id"], "architecture":image_info["Architecture"], "user":image_info["Config"]["User"]}, indent=2)+"\n")
    paths = sorted([*pathlib.Path("internal").rglob("*.go"), *pathlib.Path("cmd").rglob("*.go"),
                    *pathlib.Path("migrations").glob("*"), *pathlib.Path("internal/ui/assets").glob("*"),
                    *pathlib.Path("internal/ui/templates").glob("*"), pathlib.Path("Dockerfile"),
                    pathlib.Path(".dockerignore"), pathlib.Path("go.mod"), pathlib.Path("go.sum")])
    source = {str(path): hashlib.sha256(path.read_bytes()).hexdigest()
              for path in paths if not path.name.endswith("_test.go")}
    (evidence / "source.json").write_text(json.dumps({
        "base_commit": command(["git", "rev-parse", "HEAD"]), "working_tree": True, "files": source
    }, indent=2) + "\n")
    env.update(ARTIFACTS_PUBLIC_URL=origins[0], ARTIFACTS_URL=origins[0])
    servers = [launch(0), launch(1)]
    check("unauthenticated liveness", all(probe(node,"/healthz") == 200 for node in range(2)))
    command(["docker", "pause", name])
    try:
        check("database outage removes readiness", probe(0,"/readyz") == 503)
        check("database outage preserves liveness", probe(0,"/healthz") == 200)
    finally:
        command(["docker", "unpause", name])
    check("readiness recovers", probe(0,"/readyz") == 200)
    command(["docker", "pause", storage])
    try:
        check("object storage outage removes readiness", probe(1,"/readyz") == 503)
        check("object storage outage preserves liveness", probe(1,"/healthz") == 200)
    finally:
        command(["docker", "unpause", storage])
    check("object storage readiness recovers", probe(1,"/readyz") == 200)
    doctor = command(["go", "run", "./examples/agent-harness", "doctor"], env=env)
    (evidence / "doctor.txt").write_text(doctor + "\n")
    command(["go", "run", "./examples/agent-harness", "verify-core", "--evidence", str(evidence / "core")], env=env)
    report = json.loads((evidence / "core/report.json").read_text())
    check("container core workflow and cleanup", report["classification"] == "none" and all(a["passed"] for a in report["assertions"]))
    for artifact in report["artifacts"]:
        data = (evidence / "core" / artifact["path"]).read_bytes()
        check("evidence integrity " + artifact["path"], len(data) == artifact["size"] and hashlib.sha256(data).hexdigest() == artifact["sha256"])
    for binary_size in args.binary_size_mib:
        binary_dir = evidence / f"binary-{binary_size}"
        print(f"Verifying {binary_size} MiB multipart commits", flush=True)
        command(["go", "run", "./examples/agent-harness", "verify-binary", "--size-mib",
                 str(binary_size), "--evidence", str(binary_dir)], env=env)
        binary_report = json.loads((binary_dir / "report.json").read_text())
        check(f"binary {binary_size} MiB workflow and cleanup", binary_report["classification"] == "none"
              and all(item["passed"] for item in binary_report["assertions"]))
        for artifact in binary_report["artifacts"]:
            data = (binary_dir / artifact["path"]).read_bytes()
            check(f"binary {binary_size} evidence integrity " + artifact["path"],
                  len(data) == artifact["size"] and hashlib.sha256(data).hexdigest() == artifact["sha256"])
    if args.binary_size_mib:
        verify_binary_clients()
    collection = "/namespaces/multi/repos"
    repo_name = "run-" + secrets.token_hex(4)
    path = collection + "/" + repo_name
    created = api(0, "POST", collection, body={"name": repo_name})
    worker = created["credential"]["plaintext"]
    secret_values.extend([worker, worker.split("?")[0]])
    git_env = dict(env, ARTIFACTS_GIT_AUTH="Authorization: Bearer " + worker,
                   GIT_CONFIG_GLOBAL="/dev/null", GIT_CONFIG_NOSYSTEM="1", GIT_TERMINAL_PROMPT="0")
    api(0, "POST", path + "/commits", worker,
               {"expected_head": "", "files": [{"path": "brief", "content": "keep"}]}, want=201)
    check("other instance reads REST publication", request(1, "GET", path + "/file?path=brief", worker) == (200, b"keep"))
    clone = scratch / "clone"
    git("clone", origins[1] + f"/git/local/multi/{repo_name}.git", str(clone))
    (clone / "git-result").write_text("Git on node 1")
    git("-C", str(clone), "add", "git-result")
    git("-C", str(clone), "-c", "user.name=Multi Check", "-c", "user.email=check@example.test",
        "-c", "commit.gpgsign=false", "commit", "-m", "Git publication")
    git("-C", str(clone), "push", "origin", "main")
    sha = git("-C", str(clone), "rev-parse", "HEAD").stdout.strip()
    check("node 0 reads node 1 Git publication", request(0, "GET", path + "/file?path=git-result", worker) == (200, b"Git on node 1"))
    def race(node):
        return request(node, "POST", path + "/commits", worker,
            {"expected_head": sha, "files": [{"path": "race", "content": str(node)}]})
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        outcomes = list(pool.map(race, range(2)))
    check("competing expected-head writes have one winner", sorted(status for status, _ in outcomes) == [201, 409])
    update = {"files": [{"path": "retry", "content": "once"}]}
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        results = list(pool.map(lambda node: api(node, "POST", path + "/commits", worker, update, "same-operation", 201), range(2)))
    check("cross-instance idempotency returns same publication", results[0] == results[1])
    check("failed and replayed writes do not add publications", len(api(1, "GET", path + "/wal")) == 4)
    if args.s3_sse:
        command(["docker", "exec", servers[1], "artifacts", "compact", "--account", "local", "--namespace", "multi", "--repo", repo_name])
        check("compaction preserves encrypted history", request(0, "GET", path + "/file?path=brief", worker) == (200, b"keep"))
    stop(servers[0])
    check("surviving instance reads acknowledged history", request(1, "GET", path + "/file?path=brief", worker) == (200, b"keep"))
    servers[0] = launch(0)
    check("cold restarted instance reads durable history", request(0, "GET", path + "/file?path=brief", worker) == (200, b"keep"))
    git("-C", str(clone), "fetch", origins[0] + f"/git/local/multi/{repo_name}.git", "main")
    check("cross-instance retry survives restart", api(0, "POST", path + "/commits", worker, update, "same-operation", 201) == results[0])
    api(1, "DELETE", path, want=202)
    check("repository cleanup visible on node 0", request(0, "GET", path)[0] == 404)
    if args.s3_sse:
        rows = [json.loads(line) for line in command(["docker", "exec", "-e", "MC_HOST_local", storage,
                "mc", "stat", "--json", "--recursive", "local/artifacts"], env=bucket_env).splitlines()]
        check("encrypted objects were written", len(rows) > 0)
        metadata_rows = [{key.lower(): value for key, value in row.get("metadata", {}).items()} for row in rows]
        (evidence / "encryption.json").write_text(json.dumps({"mode": args.s3_sse, "objects_checked": len(rows),
            "provider": "owned MinIO with local test key", "aws_kms_verified": False,
            "requested_key_id": env["S3_SSE_KMS_KEY_ID"],
            "observed": [{"mode": metadata.get("x-amz-server-side-encryption"),
                          "key_id": metadata.get("x-amz-server-side-encryption-aws-kms-key-id")}
                         for metadata in metadata_rows]}, indent=2) + "\n")
        for metadata in metadata_rows:
            check("stored object encryption matches configuration", metadata.get("x-amz-server-side-encryption") == args.s3_sse)
            if args.s3_sse == "aws:kms":
                check("stored object uses requested KMS key", metadata.get("x-amz-server-side-encryption-aws-kms-key-id") == "arn:aws:kms:artifacts-test")
    for server in servers:
        stop(server)
    check_neighbor()
    completed = True
    print(f"PASS: {len(checks)} container assertions", flush=True)
finally:
    for container in reversed(containers):
        subprocess.run(["docker", "rm", "-f", "-v", container], capture_output=True)
    cleanup = all(subprocess.run(["docker", "inspect", container], capture_output=True).returncode != 0 for container in containers)
    for volume in volumes:
        cleanup = subprocess.run(["docker", "volume", "rm", volume], capture_output=True).returncode == 0 and cleanup
    cleanup = subprocess.run(["docker", "network", "rm", network], capture_output=True).returncode == 0 and cleanup
    shutil.rmtree(scratch)
    checks.append({"name": "owned containers, volumes, network, and scratch removed", "passed": cleanup})
    (evidence / "checks.json").write_text(json.dumps({"passed": completed and cleanup and all(c["passed"] for c in checks), "checks": checks}, indent=2) + "\n")
    for file in evidence.rglob("*"):
        if file.is_file():
            data = file.read_text()
            if any(value in data for value in secret_values):
                raise AssertionError("evidence contains credential")
