#!/usr/bin/env python3
"""Operate the independent push soak; secrets stay in private local files."""
import argparse
import datetime
import json
import os
from pathlib import Path
import secrets
import subprocess
import urllib.error
import urllib.parse
import urllib.request

PRIVATE = Path.home() / '.config/ding-cloud/push-soak'
URL = 'https://ding-cloud-push-soak.zuchka.workers.dev'
REPO = Path(__file__).resolve().parents[3]

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


def save(path, value):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(value, f, indent=2)
        f.flush()
        os.fsync(f.fileno())


def request(path, data=None, method=None):
    path_secret = PRIVATE / 'worker-secrets.json'
    if path_secret.stat().st_mode & 0o077:
        raise SystemExit('Worker secret file must be private.')
    token = json.loads(path_secret.read_text())['ADMIN_TOKEN']
    req = urllib.request.Request(URL + path, data=None if data is None else json.dumps(data).encode(),
        method=method, headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json', 'User-Agent': 'ding-push-soak-operator/1.0'})
    try:
        with urllib.request.build_opener(NoRedirect).open(req, timeout=30) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        raise SystemExit(f'Runner returned HTTP {e.code}: {e.read(4096).decode()}')


def export(state):
    c = state['config']
    if not c:
        raise SystemExit('No run has started.')
    root = PRIVATE / 'evidence'
    cursor = None
    while True:
        query = {'prefix': c['runId'] + '/'}
        if cursor:
            query['cursor'] = cursor
        page = request('/admin/list?' + urllib.parse.urlencode(query))
        for obj in page['objects']:
            key = obj['key']
            if not key.startswith(c['runId'] + '/') or '..' in Path(key).parts:
                raise SystemExit('Invalid evidence key.')
            path = root / key
            if path.exists() and not key.endswith('/latest.json'):
                continue
            save(path, request('/admin/object?' + urllib.parse.urlencode({'key': key})))
        cursor = page['cursor']
        if not cursor:
            break
    samples = [json.loads(p.read_text()) for p in (root / c['runId'] / c['mode'] / 'samples').glob('*.json')]
    script = "import {report} from './deploy/soak/worker/src/push-core.mjs';let s='';for await(const c of process.stdin)s+=c;const d=JSON.parse(s);console.log(JSON.stringify(report(d.config,d.samples,d.now)));"
    result = subprocess.check_output(['node', '--input-type=module', '-e', script], cwd=REPO,
        input=json.dumps({'config': c, 'samples': samples, 'now': state['now']}), text=True)
    report = json.loads(result)
    if state.get('stop'):
        report.update(verdict='fail', stop=state['stop'])
    save(root / c['runId'] / c['mode'] / 'report.json', report)
    return report


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['status', 'export', 'warmup', 'restart', 'start', 'stop'])
    p.add_argument('--application-revision')
    args = p.parse_args()
    state = request('/admin/status')
    save(PRIVATE / 'status.json', state)
    if args.action in ('warmup', 'restart', 'start'):
        now = state['now']
        if not state.get('lastCron') or now - state['lastCron']['at'] > 120000:
            raise SystemExit('A recent actual scheduled invocation is required.')
        mode = 'measure' if args.action == 'start' else 'warmup'
        c = state.get('config')
        restarting = args.action == 'restart'
        if restarting:
            if not c or not state.get('stop'):
                raise SystemExit('Restart requires an explicitly stopped run.')
            export(state)  # Preserve the stopped attempt before changing the active pointer.
        if mode == 'measure' and (not c or c['mode'] != 'warmup'):
            raise SystemExit('Inspect existing state; measurement requires warmup.')
        if mode == 'warmup' and c and not restarting:
            raise SystemExit('A run already exists; do not replace it.')
        revision = args.application_revision or (c or {}).get('applicationRevision')
        config = {'mode': mode, 'runId': c['runId'] if c and not restarting else 'push-' + secrets.token_hex(8),
            'watchId': 'push-soak-20261012', 'epoch': (now // 60000 + 1) * 60000,
            'applicationRevision': revision,
            'observerRevision': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=REPO, text=True).strip()}
        if restarting:
            config['previousRunId'] = c['runId']
        result = request('/admin/start', config, 'PUT')
        save(PRIVATE / ('manifest-' + mode + '.json'), result)
        print(json.dumps(result))
    elif args.action == 'export':
        print(json.dumps(export(state)))
    elif args.action == 'stop':
        print(json.dumps(request('/admin/stop', {}, 'POST')))
    else:
        latest = state.get('latest') or {}
        print(json.dumps({'config': state.get('config'), 'lastCron': state.get('lastCron'), 'stop': state.get('stop'),
            'latest': {k: latest.get(k) for k in ['scheduledAt', 'pass', 'health', 'telemetryPass', 'failureStreak']}}))

if __name__ == '__main__':
    main()
