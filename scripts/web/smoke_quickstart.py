#!/usr/bin/env python3
"""Prove the documented first watch against an isolated daemon and empty store."""
import json
from pathlib import Path
import signal
import subprocess
import tempfile
import time
from urllib.request import Request, urlopen

ROOT=Path(__file__).resolve().parents[2]
with tempfile.TemporaryDirectory(prefix='ding-docs-smoke-') as temp:
    work=Path(temp);binary=work/'ding';state=work/'state'
    subprocess.run(['go','build','-o',str(binary),'./cmd/ding'],cwd=ROOT,check=True)
    def cli(*args):
        result=subprocess.run([str(binary),*args,'--state-dir',str(state),'--json'],cwd=ROOT,text=True,capture_output=True)
        if result.returncode:raise RuntimeError(f'CLI operation failed: {args[0]}: {result.stderr}')
        return json.loads(result.stdout)['data']
    for path in [ROOT/'ding.yaml.example',*sorted((ROOT/'examples/watches').glob('*.yaml'))]:cli('validate',str(path))
    fixture=cli('test','examples/watches/api-health.yaml','--events','testdata/watches/api-health.jsonl')
    assert [event['type'] for event in fixture['events']]==['firing','recovered']
    def start():
        process=subprocess.Popen([str(binary),'daemon','--state-dir',str(state),'--listen','127.0.0.1:0'],cwd=ROOT,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        for _ in range(150):
            if process.poll() is not None:raise RuntimeError('Daemon exited during startup')
            try:
                url=json.loads((state/'connection.json').read_text())['url']
                with urlopen(url+'/health',timeout=.5) as response:
                    if response.status==200:return process,url
            except (OSError,ValueError):pass
            time.sleep(.05)
        process.terminate();process.wait(timeout=10);raise RuntimeError('Daemon did not start')
    process,url=start()
    try:
        cli('apply','ding.yaml.example','--dry-run')
        assert cli('watch','list')==[],'Dry-run changed the store'
        cli('apply','ding.yaml.example')
        token=json.loads((state/'tokens.json').read_text())['ingest']
        def push(value,key):
            request=Request(url+'/v1/ingest/latency',data=json.dumps({'latency_ms':value}).encode(),headers={'Authorization':'Bearer '+token,'Content-Type':'application/json','Idempotency-Key':key})
            with urlopen(request,timeout=5) as response:
                assert response.status==202
                return json.load(response)
        first=push(350,'tutorial-high')['data'];duplicate=push(350,'tutorial-high')['data']
        assert duplicate['duplicate'] and (duplicate['first'],duplicate['last'])==(first['first'],first['last'])
        push(100,'tutorial-recovery')
        events=cli('events','--watch','latency')['events']
        assert [event['type'] for event in events if event['type'] in ('firing','recovered')]==['firing','recovered']
        proof=cli('events','inspect',next(event['id'] for event in events if event['type']=='firing'))
        evidence=work/'evidence.json';evidence.write_text(json.dumps(proof));cli('replay',str(evidence))
        before=[event['id'] for event in events]
        process.send_signal(signal.SIGTERM);process.wait(timeout=10)
        (state/'connection.json').unlink()
        process,url=start()
        after=[event['id'] for event in cli('events','--watch','latency')['events']]
        assert set(before).issubset(after),'Restart lost events'
        cli('doctor');cli('watch','pause','latency')
        print('Quickstart passed: validate, dry-run, apply, idempotent push, firing, recovery, evidence replay, restart, doctor, pause')
    finally:
        if process.poll() is None:
            process.send_signal(signal.SIGTERM);process.wait(timeout=10)
