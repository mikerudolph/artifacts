import hashlib, json, os, pathlib, secrets, shutil, socket, subprocess, tempfile, time, urllib.request, urllib.error
root = pathlib.Path.cwd()
evidence = root / 'reviews/evidence/interface-changes'
evidence.mkdir(parents=True, exist_ok=True)
scratch = pathlib.Path(tempfile.mkdtemp(prefix='artifacts-interface-'))
container = 'artifacts-interface-' + secrets.token_hex(5)
server = None
checks = []
password = secrets.token_hex(20)
token = secrets.token_hex(24)
env = dict(os.environ, POSTGRES_PASSWORD=password)

def command(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, text=True, **kwargs).stdout.strip()

def check(name, condition):
    if not condition: raise AssertionError(name)
    checks.append({'name':name, 'passed':True})

def request(method, path, credential=token, body=None, key=None):
    headers = {'Authorization':'Bearer '+credential}
    if body is not None: headers['Content-Type']='application/json'
    if key: headers['Idempotency-Key']=key
    req = urllib.request.Request(origin+base+path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as response: return response.status, response.read()
    except urllib.error.HTTPError as error: return error.code, error.read()

def api(method,path,credential=token,body=None,key=None,want=200):
    status, data = request(method,path,credential,body,key)
    if status!=want: raise AssertionError(f'{method} {path}: HTTP {status}, expected {want}')
    return json.loads(data)['result']

def launch():
    log = open(scratch/'server.log','ab')
    process = subprocess.Popen([str(scratch/'artifacts'),'serve'], env=env, stdout=log, stderr=log)
    log.close()
    for _ in range(100):
        if process.poll() is not None: raise RuntimeError('owned server failed to start')
        try:
            if request('GET','/namespaces')[0] == 200: return process
        except OSError: pass
        time.sleep(.1)
    raise RuntimeError('owned server readiness timeout')

try:
    command(['docker','run','-d','--rm','--name',container,'-e','POSTGRES_PASSWORD','-e','POSTGRES_USER=artifacts','-e','POSTGRES_DB=artifacts','-p','127.0.0.1::5432','postgres:16-alpine'],env=env)
    port = command(['docker','port',container,'5432/tcp']).rsplit(':',1)[1]
    for _ in range(100):
        if subprocess.run(['docker','exec',container,'pg_isready','-U','artifacts'],capture_output=True).returncode==0: break
        time.sleep(.1)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1',0)); http_port=sock.getsockname()[1]
    origin=f'http://127.0.0.1:{http_port}'
    base='/client/v4/accounts/local/artifacts'
    env.update(DATABASE_URL=f'postgres://artifacts:{password}@127.0.0.1:{port}/artifacts?sslmode=disable',ARTIFACTS_HTTP_ADDR=f'127.0.0.1:{http_port}',ARTIFACTS_PUBLIC_URL=origin,ARTIFACTS_URL=origin,ARTIFACTS_ACCOUNT='local',ARTIFACTS_DEFAULT_ACCOUNT='local',ARTIFACTS_AUTH='token',ARTIFACTS_API_TOKEN=token,ARTIFACTS_STORAGE='fs',ARTIFACTS_DATA_DIR=str(scratch/'objects'),ARTIFACTS_CACHE_DIR=str(scratch/'cache'))
    command(['go','build','-o',str(scratch/'artifacts'),'./cmd/artifacts'])
    server=launch()
    doctor = command(['go','run','./examples/agent-harness','doctor'],env=env)
    (evidence/'doctor.txt').write_text(doctor+'\n')
    command(['go','run','./examples/agent-harness','verify-core','--evidence',str(evidence/'core')],env=env)
    report=json.loads((evidence/'core/report.json').read_text())
    check('core harness classification',report['classification']=='none')
    check('core harness assertions',all(a['passed'] for a in report['assertions']))
    for artifact in report['artifacts']:
        data=(evidence/'core'/artifact['path']).read_bytes()
        check('evidence integrity '+artifact['path'],len(data)==artifact['size'] and hashlib.sha256(data).hexdigest()==artifact['sha256'])
    collection='/namespaces/interface-check/repos'
    name='run-'+secrets.token_hex(4)
    path=collection+'/'+name
    body={'name':name,'issue_credential':False}
    created=api('POST',collection,body=body,key='create')
    credential=api('POST','/namespaces/interface-check/credentials',body={'repo':name,'scope':'write','ttl':3600})
    worker=credential['plaintext']
    git_env=dict(env,ARTIFACTS_GIT_AUTH='Authorization: Bearer '+worker)
    def git(*args): return command(['git','--config-env=http.extraHeader=ARTIFACTS_GIT_AUTH',*args],env=git_env)
    seed=api('POST',path+'/commits',worker,{'expected_head':'','files':[{'path':'brief','content':'keep'},{'path':'report','content':'draft'}]},key='seed',want=201)
    clone=scratch/'clone';git('clone',created['remote'],str(clone))
    (clone/'git-result').write_text('published by Git')
    git('-C',str(clone),'add','git-result')
    git('-C',str(clone),'-c','user.name=Interface Check','-c','user.email=check@example.test','-c','commit.gpgsign=false','commit','-m','Git publication')
    git('-C',str(clone),'push','origin','main')
    sha=git('-C',str(clone),'rev-parse','HEAD')
    stale={'expected_head':seed['sha'],'files':[{'path':'report','content':'done'}]}
    status,data=request('POST',path+'/commits',worker,stale)
    check('REST precondition detects Git push',status==409 and json.loads(data)['errors'][0]['current_head']==sha)
    update={'expected_head':sha,'files':[{'path':'report','content':'done'}]}
    published=api('POST',path+'/commits',worker,update,key='report',want=201)
    check('REST preserves Git-authored file',request('GET',path+'/file?path=git-result',worker)==(200,b'published by Git'))
    check('REST preserves untouched input',request('GET',path+'/file?path=brief',worker)==(200,b'keep'))
    git('-C',str(clone),'pull','--ff-only')
    check('Git reads REST update',(clone/'report').read_text()=='done')
    server.terminate();server.wait(timeout=10);server=None
    server=launch()
    check('create replay after actual server restart',api('POST',collection,body=body,key='create')==created)
    check('commit replay after actual server restart',api('POST',path+'/commits',worker,update,key='report',want=201)==published)
    check('replay did not publish another WAL record',len(api('GET',path+'/wal'))==3)
    check('changed retry is a conflict',request('POST',path+'/commits',worker,stale,'report')[0]==409)
    read=api('POST','/namespaces/interface-check/credentials',body={'repo':name,'scope':'read','ttl':3600})
    check('read credential can read REST',request('GET',path+'/file?path=brief',read['plaintext'])==(200,b'keep'))
    check('read credential cannot write REST',request('POST',path+'/commits',read['plaintext'],update)[0]==403)
    check('repository credential cannot create repositories',request('POST',collection,worker,{'name':'forbidden'})[0]==401)
    api('DELETE','/namespaces/interface-check/credentials/'+credential['id'])
    check('revocation blocks REST',request('GET',path+'/file?path=brief',worker)[0]==401)
    result=subprocess.run(['git','--config-env=http.extraHeader=ARTIFACTS_GIT_AUTH','-C',str(clone),'fetch'],env=git_env,capture_output=True)
    check('revocation blocks Git',result.returncode!=0)
    api('DELETE',path,want=202)
    check('custom repository cleanup',request('GET',path)[0]==404)
    for file in evidence.rglob('*'):
        if file.is_file():
            text=file.read_text()
            for secret in [token,password,worker,worker.split('?')[0],read['plaintext']]:
                if secret in text: raise AssertionError('evidence contains credential')
    (evidence/'interface-checks.json').write_text(json.dumps({'passed':True,'checks':checks},indent=2)+'\n')
    print(f'PASS: {len(checks)} live checks; evidence: {evidence}',flush=True)
finally:
    if server is not None:
        server.terminate();server.wait(timeout=10)
    subprocess.run(['docker','stop',container],capture_output=True)
    shutil.rmtree(scratch)
