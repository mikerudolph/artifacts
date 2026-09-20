import argparse
import concurrent.futures
import hashlib
import json
import os
import pathlib
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
args = parser.parse_args()
evidence = pathlib.Path(args.evidence).resolve()
evidence.mkdir(parents=True, exist_ok=True)
scratch = pathlib.Path(tempfile.mkdtemp(prefix="artifacts-container-"))
prefix = "artifacts-container-" + secrets.token_hex(5)
containers, checks, secret_values = [], [], []
network = prefix + "-network"
completed = False
servers = []
base = "/client/v4/accounts/local/artifacts"
token, password, access, secret = [secrets.token_hex(24) for _ in range(4)]
secret_values.extend([token, password, access, secret])
env = dict(os.environ, POSTGRES_PASSWORD=password, MINIO_ROOT_USER=access,
           MINIO_ROOT_PASSWORD=secret, AWS_ACCESS_KEY_ID=access, AWS_SECRET_ACCESS_KEY=secret)
env.pop("AWS_SESSION_TOKEN", None)

def command(args, **kwargs):
    result = subprocess.run(args, capture_output=True, text=True, **kwargs)
    if result.returncode:
        raise RuntimeError("command failed: " + args[0])
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
            "ARTIFACTS_HTTP_ADDR", "ARTIFACTS_PUBLIC_URL", "ARTIFACTS_STORAGE", "S3_BUCKET",
            "S3_REGION", "S3_ENDPOINT", "S3_USE_PATH_STYLE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"]
    command(["docker", "run", "-d", "--name", name, "--network", network, "--read-only",
             "--cap-drop=ALL", "--security-opt=no-new-privileges", "--tmpfs",
             "/var/cache/artifacts:rw,uid=10001,gid=10001,mode=0700", "-p",
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

def setup(*arguments, token_value=token, expected=0):
    setup_env = dict(env, ARTIFACTS_BOOTSTRAP_TOKEN=token_value)
    result = subprocess.run(["docker", "run", "--rm", "--read-only", "--network", network,
                             "-e", "DATABASE_URL", "-e", "ARTIFACTS_BOOTSTRAP_TOKEN",
                             args.image, *arguments], env=setup_env, capture_output=True, text=True)
    check("setup command " + arguments[0], result.returncode == expected)
    check("setup output redacts credentials", not any(value in result.stdout + result.stderr for value in secret_values))

def git(*args, expect_success=True):
    result = subprocess.run(["git", "--config-env=http.extraHeader=ARTIFACTS_GIT_AUTH", *args],
                            env=git_env, capture_output=True, text=True)
    if expect_success and result.returncode:
        raise AssertionError("Git operation failed")
    return result

try:
    command(["docker", "network", "create", network])
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
             "-e", "MINIO_ROOT_USER", "-e", "MINIO_ROOT_PASSWORD", "-p", "127.0.0.1::9000",
             "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e",
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
        ARTIFACTS_PUBLIC_URL=origins[0], ARTIFACTS_URL=origins[0], ARTIFACTS_ACCOUNT="local",
        ARTIFACTS_DEFAULT_ACCOUNT="local", ARTIFACTS_AUTH="token", ARTIFACTS_API_TOKEN=token,
        ARTIFACTS_STORAGE="s3", S3_BUCKET="artifacts", S3_REGION="us-east-1",
        S3_ENDPOINT=f"http://{storage}:9000", S3_USE_PATH_STYLE="true", S3_PREFIX="")
    setup("bootstrap", "--account", "local", expected=1)
    setup("migrate")
    setup("migrate")
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        list(pool.map(lambda _: setup("bootstrap", "--account", "local"), range(2)))
    setup("bootstrap", "--account", "other", expected=1)
    state = command(["docker", "exec", "-i", name, "psql", "-U", "artifacts", "-d", "artifacts", "-At"],
        input="SELECT count(*) FROM api_tokens; SELECT count(*) FROM accounts WHERE id='other';")
    check("bootstrap retries preserve one token and rejected account rolls back", state == "1\n0")
    runtime_password = secrets.token_hex(24)
    secret_values.append(runtime_password)
    runtime_dsn = f"postgres://runtime:{runtime_password}@{name}/artifacts?sslmode=disable"
    command(["docker", "exec", "-i", name, "psql", "-U", "artifacts", "-d", "artifacts", "-v", "ON_ERROR_STOP=1"],
        input=f"CREATE ROLE runtime LOGIN PASSWORD '{runtime_password}'; GRANT CONNECT ON DATABASE artifacts TO runtime; GRANT USAGE ON SCHEMA public TO runtime; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO runtime; GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO runtime;")
    permission = command(["docker", "exec", "-i", name, "psql", "-U", "artifacts", "-d", "artifacts", "-At"],
        input="SELECT has_schema_privilege('runtime', 'public', 'CREATE');")
    check("runtime role cannot create schema objects", permission == "f")
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
    stop(servers[0])
    check("surviving instance reads acknowledged history", request(1, "GET", path + "/file?path=brief", worker) == (200, b"keep"))
    servers[0] = launch(0)
    check("cold restarted instance reads durable history", request(0, "GET", path + "/file?path=brief", worker) == (200, b"keep"))
    git("-C", str(clone), "fetch", origins[0] + f"/git/local/multi/{repo_name}.git", "main")
    check("cross-instance retry survives restart", api(0, "POST", path + "/commits", worker, update, "same-operation", 201) == results[0])
    api(1, "DELETE", path, want=202)
    check("repository cleanup visible on node 0", request(0, "GET", path)[0] == 404)
    for server in servers:
        stop(server)
    completed = True
    print(f"PASS: {len(checks)} container assertions", flush=True)
finally:
    for container in reversed(containers):
        subprocess.run(["docker", "rm", "-f", "-v", container], capture_output=True)
    cleanup = all(subprocess.run(["docker", "inspect", container], capture_output=True).returncode != 0 for container in containers)
    cleanup = subprocess.run(["docker", "network", "rm", network], capture_output=True).returncode == 0 and cleanup
    shutil.rmtree(scratch)
    checks.append({"name": "owned containers, network, and scratch removed", "passed": cleanup})
    (evidence / "checks.json").write_text(json.dumps({"passed": completed and cleanup and all(c["passed"] for c in checks), "checks": checks}, indent=2) + "\n")
    for file in evidence.rglob("*"):
        if file.is_file():
            data = file.read_text()
            if any(value in data for value in secret_values):
                raise AssertionError("evidence contains credential")
