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
evidence = root / "reviews/evidence/multi-instance"
scratch = pathlib.Path(tempfile.mkdtemp(prefix="artifacts-multi-"))
prefix = "artifacts-multi-" + secrets.token_hex(5)
containers, processes, checks, secret_values = [], [], [], []
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

def launch(node):
    node_env = dict(env, ARTIFACTS_HTTP_ADDR=origins[node].removeprefix("http://"),
                    ARTIFACTS_CACHE_DIR=str(scratch / f"cache-{node}"))
    with open(scratch / f"server-{node}.log", "ab") as log:
        process = subprocess.Popen([str(scratch / "artifacts"), "serve"],
                                   env=node_env, stdout=log, stderr=log)
    processes.append(process)
    for _ in range(200):
        if process.poll() is not None:
            raise RuntimeError("owned server failed to start")
        try:
            if request(node, "GET", "/namespaces")[0] == 200:
                return process
        except OSError:
            pass
        time.sleep(.1)
    raise RuntimeError("owned server readiness timeout")

def stop(process):
    if process.poll() is None:
        process.terminate()
        process.wait(timeout=15)

def git(*args, expect_success=True):
    result = subprocess.run(["git", "--config-env=http.extraHeader=ARTIFACTS_GIT_AUTH", *args],
                            env=git_env, capture_output=True, text=True)
    if expect_success and result.returncode:
        raise AssertionError("Git operation failed")
    return result

def compact(node, name):
    node_env = dict(env, ARTIFACTS_CACHE_DIR=str(scratch / f"compact-{node}"))
    command([str(scratch / "artifacts"), "compact", "--account", "local",
             "--namespace", "multi", "--repo", name], env=node_env)

try:
    name = prefix + "-postgres"
    command(["docker", "run", "-d", "--rm", "--name", name, "-e", "POSTGRES_PASSWORD",
             "-e", "POSTGRES_USER=artifacts", "-e", "POSTGRES_DB=artifacts",
             "-p", "127.0.0.1::5432", "postgres:16-alpine"], env=env)
    containers.append(name)
    db_port = command(["docker", "port", name, "5432/tcp"]).rsplit(":", 1)[1]
    for _ in range(200):
        if subprocess.run(["docker", "exec", name, "pg_isready", "-U", "artifacts"],
                          capture_output=True).returncode == 0:
            break
        time.sleep(.1)
    storage = prefix + "-minio"
    command(["docker", "run", "-d", "--rm", "--name", storage,
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
    origin = f"http://127.0.0.1:{free_port()}"
    env.update(DATABASE_URL=f"postgres://artifacts:{password}@127.0.0.1:{db_port}/artifacts?sslmode=disable",
        ARTIFACTS_PUBLIC_URL=origin, ARTIFACTS_URL=origin, ARTIFACTS_ACCOUNT="local",
        ARTIFACTS_DEFAULT_ACCOUNT="local", ARTIFACTS_AUTH="token", ARTIFACTS_API_TOKEN=token,
        ARTIFACTS_STORAGE="s3", S3_BUCKET="artifacts", S3_REGION="us-east-1",
        S3_ENDPOINT=f"http://127.0.0.1:{s3_port}", S3_USE_PATH_STYLE="true", S3_PREFIX="")
    command(["go", "build", "-o", str(scratch / "artifacts"), "./cmd/artifacts"])
    paths = sorted([*pathlib.Path("internal").rglob("*.go"), *pathlib.Path("cmd").rglob("*.go"),
                    pathlib.Path("go.mod"), pathlib.Path("go.sum")])
    source = {str(path): hashlib.sha256(path.read_bytes()).hexdigest()
              for path in paths if not path.name.endswith("_test.go")}
    (evidence / "source.json").write_text(json.dumps({
        "base_commit": command(["git", "rev-parse", "HEAD"]), "working_tree": True, "files": source
    }, indent=2) + "\n")
    proxy_source = """package main
import ("log";"net/http";"net/http/httputil";"net/url";"os";"sync/atomic")
func main() {
 var count atomic.Uint64
 targets := []*httputil.ReverseProxy{}
 for _, address := range os.Args[2:] { u,_ := url.Parse(address); targets=append(targets,httputil.NewSingleHostReverseProxy(u)) }
 log.Fatal(http.ListenAndServe(os.Args[1], http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  index := (count.Add(1)-1)%uint64(len(targets))
  log.Printf("node=%d method=%s path=%s",index,r.Method,r.URL.Path)
  targets[index].ServeHTTP(w,r)
 })))
}
"""
    (scratch / "proxy.go").write_text(proxy_source)
    command(["go", "build", "-o", str(scratch / "proxy"), str(scratch / "proxy.go")])
    servers = [launch(0), launch(1)]
    with open(scratch / "proxy.log", "w") as log:
        proxy = subprocess.Popen([str(scratch / "proxy"), origin.removeprefix("http://"), *origins],
                                 stdout=log, stderr=log)
    processes.append(proxy)
    for _ in range(100):
        try:
            with socket.create_connection(("127.0.0.1", int(origin.rsplit(":", 1)[1])), timeout=1):
                break
        except OSError:
            time.sleep(.1)
    doctor = command(["go", "run", "./examples/agent-harness", "doctor"], env=env)
    (evidence / "doctor.txt").write_text(doctor + "\n")
    command(["go", "run", "./examples/agent-harness", "verify-core", "--evidence", str(evidence / "core")], env=env)
    report = json.loads((evidence / "core/report.json").read_text())
    check("round-robin core workflow and cleanup", report["classification"] == "none" and all(a["passed"] for a in report["assertions"]))
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
    seed = api(0, "POST", path + "/commits", worker,
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
    fork_name = repo_name + "-fork"
    fork = api(1, "POST", path + "/fork", body={"name": fork_name})
    fork_path = collection + "/" + fork_name
    api(0, "POST", path + "/commits", worker, {"files": [{"path": "brief", "content": "changed"}]}, want=201)
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        list(pool.map(lambda node: compact(node, repo_name), range(2)))
    check("compaction creates no publication event", len(api(1, "GET", path + "/events")["events"]) == 5)
    check("fork retains captured parent version", request(0, "GET", fork_path + "/file?path=brief") == (200, b"keep"))
    stop(servers[0])
    check("surviving instance reads acknowledged history", request(1, "GET", path + "/file?path=brief", worker) == (200, b"changed"))
    shutil.rmtree(scratch / "cache-0")
    servers[0] = launch(0)
    check("cold restarted instance reads checkpoint", request(0, "GET", path + "/file?path=brief", worker) == (200, b"changed"))
    git("-C", str(clone), "fetch", origins[0] + f"/git/local/multi/{repo_name}.git", "main")
    check("cross-instance retry survives restart", api(0, "POST", path + "/commits", worker, update, "same-operation", 201) == results[0])
    clones = [scratch / f"contender-{node}" for node in range(2)]
    for node, work in enumerate(clones):
        git("clone", origins[node] + f"/git/local/multi/{repo_name}.git", str(work))
        (work / "contender").write_text(str(node))
        git("-C", str(work), "add", "contender")
        git("-C", str(work), "-c", "user.name=Multi Check", "-c", "user.email=check@example.test",
            "-c", "commit.gpgsign=false", "commit", "-m", "Competing Git write")
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        pushes = list(pool.map(lambda work: git("-C", str(work), "push", "origin", "main", expect_success=False), clones))
    check("competing Git pushes have one winner", sum(result.returncode == 0 for result in pushes) == 1)
    winner = str(next(node for node, result in enumerate(pushes) if result.returncode == 0)).encode()
    check("both instances read the winning Git push", all(request(node, "GET", path + "/file?path=contender", worker) == (200, winner) for node in range(2)))
    api(1, "DELETE", "/namespaces/multi/credentials/" + created["credential"]["id"])
    check("revocation on node 1 blocks node 0 REST", request(0, "GET", path + "/file?path=brief", worker)[0] == 401)
    check("revocation blocks Git on node 0", git("ls-remote", origins[0] + f"/git/local/multi/{repo_name}.git", expect_success=False).returncode != 0)
    api(0, "DELETE", path, want=202)
    check("deletion hides repository on node 1", request(1, "GET", path)[0] == 404)
    check("fork survives parent deletion", request(1, "GET", fork_path + "/file?path=brief") == (200, b"keep"))
    api(1, "DELETE", fork_path, want=202)
    check("fork cleanup visible on node 0", request(0, "GET", fork_path)[0] == 404)
    routing = (scratch / "proxy.log").read_text()
    check("round-robin requests reached both instances", all(f"node={node} " in routing for node in range(2)))
    (evidence / "routing.log").write_text(routing)
    completed = True
    print(f"PASS: {len(checks)} multi-instance assertions", flush=True)
finally:
    for process in reversed(processes):
        stop(process)
    for container in reversed(containers):
        subprocess.run(["docker", "stop", container], capture_output=True)
    cleanup = all(process.poll() is not None for process in processes)
    for container in containers:
        cleanup = cleanup and subprocess.run(["docker", "inspect", container], capture_output=True).returncode != 0
    shutil.rmtree(scratch)
    checks.append({"name": "owned processes, containers, and scratch removed", "passed": cleanup})
    (evidence / "checks.json").write_text(json.dumps({"passed": completed and cleanup and all(c["passed"] for c in checks), "checks": checks}, indent=2) + "\n")
    for file in evidence.rglob("*"):
        if file.is_file():
            data = file.read_text()
            if any(value in data for value in secret_values):
                raise AssertionError("evidence contains credential")
