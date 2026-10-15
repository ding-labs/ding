"""Private control plane for one disposable Go test process at a time."""
import base64
import hmac
import json
import os
import socket
from pathlib import Path
import subprocess
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from railway_db import bootstrap, sample

ROOT = Path('/data/ding-lab')
JOBS = {}
LOCK = threading.Lock()


class PrivateHTTPServer(ThreadingHTTPServer):
    address_family = socket.AF_INET6


def resources():
    root = Path('/sys/fs/cgroup')
    return {'at': time.time(), 'cpu': dict(line.split() for line in (root/'cpu.stat').read_text().splitlines()),
            'memoryBytes': int((root/'memory.current').read_text()),
            'cpuMax': (root/'cpu.max').read_text().strip(), 'memoryMax': (root/'memory.max').read_text().strip()}


def observe(job):
    with (job['directory']/'resources.jsonl').open('x') as evidence:
        while job['process'].poll() is None:
            try:
                value = resources()
                value['database'] = sample(ADMIN)
                evidence.write(json.dumps(value)+'\n'); evidence.flush()
            except Exception:
                evidence.write(json.dumps({'at':time.time(), 'observerError':True})+'\n'); evidence.flush()
            time.sleep(5)


def start_case(request):
    cases = json.loads((Path(__file__).parent/'cases.json').read_text())
    config = next((c.copy() for c in cases if c['Name'] == request.get('case')), None)
    if config is None or set(request)-{'case','rate','seconds','pool','seed','workspaces'}:
        raise ValueError('unknown case or option')
    config.update(Rate=int(request.get('rate',config['Rate'])), Seconds=int(request.get('seconds',config['Seconds'])),
                  Backend='postgres', PoolLimit=int(request.get('pool',4)), QueryMode='exec', Profiles=False)
    if config['PoolLimit'] not in (2,4,8,16,32):
        raise ValueError('unsupported connection pool size')
    config['ShuffleSeed'] = int(request.get('seed',0))
    if not 0 <= config['ShuffleSeed'] < 2**63:
        raise ValueError('unsupported schedule seed')
    if request.get('workspaces') is not None:
        config['Workspaces'] = int(request['workspaces'])
        if not 1 <= config['Workspaces'] <= 100:
            raise ValueError('unsupported workspace count')
    if not 1 <= config['Rate'] <= 1000 or not 1 <= config['Seconds'] <= 1800 or config['Rate']*config['Seconds'] > 1000000:
        raise ValueError('case exceeds bounded test envelope')
    with LOCK:
        if any(j['process'].poll() is None for j in JOBS.values()):
            raise ValueError('another case is running')
        identity = uuid.uuid4().hex
        directory = ROOT/'runs'/identity
        directory.mkdir(parents=True, mode=0o700)
        (directory/'config.json').write_text(json.dumps(config))
        env = dict(os.environ, DING_SCALE_CONFIG=str(directory/'config.json'), DING_SCALE_OUTPUT=str(directory),
                   DING_SCALE_EXTERNAL='railway', TMPDIR=str(directory))
        with (directory/'run.log').open('x') as log:
            process = subprocess.Popen(['/opt/cloud.test','-test.run=^TestLocalPushScale$','-test.v','-test.timeout=70m'],
                                       env=env, stdout=log, stderr=subprocess.STDOUT)
        job = {'process':process, 'directory':directory}
        JOBS[identity] = job
        threading.Thread(target=observe,args=(job,),daemon=True).start()
    deadline = time.monotonic()+180
    while not (directory/'bootstrap.json').exists():
        if process.poll() is not None or time.monotonic() >= deadline:
            if process.poll() is None: process.terminate()
            raise RuntimeError('fixture setup failed; inspect its private run log')
        time.sleep(.2)
    # bootstrap is written by Go before it accepts start. Retry partial JSON.
    while True:
        try:
            value = (directory/'bootstrap.json').read_bytes()
            json.loads(value)
            break
        except json.JSONDecodeError:
            if time.monotonic() >= deadline: raise
            time.sleep(.1)
    return {'id':identity, 'config':config, 'bootstrap':base64.b64encode(value).decode('ascii')}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def respond(self, code, value):
        raw = json.dumps(value).encode()
        self.send_response(code); self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(raw))); self.end_headers(); self.wfile.write(raw)

    def authorized(self):
        return hmac.compare_digest(self.headers.get('Authorization',''), 'Bearer '+os.environ['DING_LAB_CONTROL'])

    def do_GET(self):
        if self.path == '/health': return self.respond(200, {'ready':True})
        if not self.authorized(): return self.respond(401, {})
        identity = self.path.removeprefix('/result/')
        job = JOBS.get(identity)
        if job is None: return self.respond(404,{})
        if job['process'].poll() is None: return self.respond(200,{'running':True})
        directory = job['directory']; report = directory/'report.json'
        (directory/'bootstrap.json').unlink(missing_ok=True)
        self.respond(200,{'running':False, 'exitCode':job['process'].returncode,
                         'report':json.loads(report.read_text()) if report.exists() else None,
                         'resources':[json.loads(line) for line in (directory/'resources.jsonl').read_text().splitlines()],
                         'settings':json.loads((ROOT/'database-settings.json').read_text())})

    def do_POST(self):
        if not self.authorized(): return self.respond(401,{})
        if self.path.startswith('/abort/'):
            job = JOBS.get(self.path.removeprefix('/abort/'))
            if job is None: return self.respond(404,{})
            if job['process'].poll() is None: job['process'].terminate()
            return self.respond(200,{'aborted':True})
        if self.path != '/case': return self.respond(404,{})
        length = int(self.headers.get('Content-Length','0'))
        if length < 1 or length > 4096: return self.respond(400,{})
        try:
            result = start_case(json.loads(self.rfile.read(length)))
        except Exception:
            return self.respond(409,{'error':'case unavailable; inspect private evidence'})
        self.respond(200,result)


if __name__ == '__main__':
    os.umask(0o077)
    try:
        ADMIN = bootstrap(ROOT)
        subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','3',
                        '-keyout',os.environ['DING_SCALE_TLS_KEY'],'-out',os.environ['DING_SCALE_TLS_CERT'],
                        '-subj','/CN=ding-scale-app.railway.internal','-addext',
                        'subjectAltName=DNS:ding-scale-app.railway.internal,IP:127.0.0.1,IP:::1'],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    except Exception:
        raise SystemExit('Lab bootstrap failed; verify private configuration')
    PrivateHTTPServer(('::',8080),Handler).serve_forever()
