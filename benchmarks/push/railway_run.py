"""Run a bounded case from the separate private Railway generator service."""
import argparse
import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import threading
import time
from urllib.request import Request, urlopen
from http.server import BaseHTTPRequestHandler, HTTPServer

from railway_app import resources


def save_bootstrap(root, encoded):
    # JSON intermediaries must not reformat the request bodies. Receipts bind
    # idempotency keys to their exact bytes, not just equivalent JSON values.
    raw = base64.b64decode(encoded, validate=True)
    json.loads(raw)
    (root/'bootstrap.json').write_bytes(raw)


def qualifies(result, config):
    report = result.get('report') or {}
    load = report.get('Load',{})
    return bool(result['exitCode']==0 and result['generatorExitCode']==0
        and load.get('firstAccepted')==config['Rate']*config['Seconds']
        and load.get('scheduleP99MS',999)<=20 and load.get('p99MS',999)<250
        and not any(load.get(k) for k in ('generatorBusy','unconfirmed','rejected','throttled','clockDiscontinuity'))
        and not report.get('Errors') and report.get('ReadErrors')==0 and not report.get('InternalObservations')
        and result['resources'] and result['generatorResources']
        and not any(s.get('observerError') for s in result['resources'])
        and report.get('Samples')
        and all('ding_cloud_ingest_seconds_count ' in s.get('Metrics','') for s in report['Samples'])
        and not sum(s['Rows']['pending'] for s in report.get('After',[]))
        and (config.get('Noop') or report.get('PostChecksVerified'))
        and (not config.get('Restart') or report.get('RestartVerified')))


def control(path, value=None):
    request = Request('http://ding-scale-app.railway.internal:8080'+path,
                      data=json.dumps(value).encode() if value is not None else None,
                      headers={'Authorization':'Bearer '+os.environ['DING_LAB_CONTROL'], 'Content-Type':'application/json'})
    with urlopen(request,timeout=240) as response:
        return json.load(response)


def run():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case',required=True)
    parser.add_argument('--rate',type=int,required=True)
    parser.add_argument('--seconds',type=int,required=True)
    parser.add_argument('--pool',type=int,choices=(2,4,8,16,32),default=4)
    parser.add_argument('--seed',type=int,default=0)
    parser.add_argument('--workspaces',type=int)
    args = parser.parse_args()
    os.umask(0o077)
    job = control('/case',vars(args))
    identity = job['id']
    root = Path('/data/ding-lab/runs')/identity
    root.mkdir(parents=True,mode=0o700)
    save_bootstrap(root,job['bootstrap'])
    (root/'config.json').write_text(json.dumps(job['config']))
    stop = threading.Event()
    def observe():
        with (root/'generator-resources.jsonl').open('x') as file:
            while not stop.is_set():
                file.write(json.dumps(resources())+'\n'); file.flush(); stop.wait(5)
    observer = threading.Thread(target=observe,daemon=True); observer.start()
    try:
        with (root/'generator.log').open('x') as log:
            process = subprocess.run(['/opt/ding-local-load','--bootstrap',str(root/'bootstrap.json'),
                                      '--output',str(root),'--rate',str(args.rate),'--seconds',str(args.seconds),
                                      '--shuffle-seed',str(args.seed)],
                                     stdout=log,stderr=subprocess.STDOUT,timeout=args.seconds+600)
        if process.returncode:
            control('/abort/'+identity,{})
        # Long cases verify every receipt twice over a real database network
        # (including after restart). Keep this outside the measured interval.
        deadline = time.monotonic()+2100
        while True:
            result = control('/result/'+identity)
            if not result['running']: break
            if time.monotonic() > deadline:
                control('/abort/'+identity,{})
                raise RuntimeError('reconciliation deadline exceeded')
            time.sleep(5)
        result['generatorExitCode'] = process.returncode
        result['id'] = identity
        result['generatorResources'] = [json.loads(line) for line in (root/'generator-resources.jsonl').read_text().splitlines()]
        (root/'result.json').write_text(json.dumps(result))
        report = result['report'] or {}
        load = report.get('Load',{})
        passed = qualifies(result,job['config'])
        summary = {'id':identity,'case':args.case,'rate':args.rate,'seconds':args.seconds,'pool':args.pool,'seed':args.seed,
                   'workspaces':job['config'].get('Workspaces',1),
                   'pass':bool(passed),'load':load,'readCount':report.get('ReadCount'),
                   'readP99MS':report.get('ReadP99MS'),'readErrors':report.get('ReadErrors'),
                   'restartVerified':report.get('RestartVerified'),'errors':report.get('Errors'),
                   'artifact':str(root/'result.json')}
        (root/'summary.json').write_text(json.dumps(summary,indent=2))
        print(json.dumps(summary),flush=True)
        return 0 if passed else 1
    except Exception:
        control('/abort/'+identity,{})
        raise
    finally:
        stop.set(); observer.join()
        (root/'bootstrap.json').unlink(missing_ok=True)


class Health(BaseHTTPRequestHandler):
    def log_message(self,*_): pass
    def do_GET(self):
        self.send_response(200 if self.path=='/health' else 404); self.end_headers()


if __name__=='__main__':
    if len(sys.argv)==1:
        HTTPServer(('0.0.0.0',8080),Health).serve_forever()
    else:
        try: sys.exit(run())
        except Exception: raise SystemExit('Private lab run failed; inspect its retained evidence')
