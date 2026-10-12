#!/usr/bin/env python3
"""Live synthetic-input check for the documented grouped push canary watch.

Usage: push-smoke.py HTTPS_INGEST_URL PRIVATE_TOKEN_FILE NEW_RECEIPT_LEDGER
The watch must project repo/failed and group by repo, with no destinations.
Creates 18 failing groups and recovers one. Never use on a real monitoring watch.
"""
import json
import os
from pathlib import Path
import secrets
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

url, token_path, ledger_path = sys.argv[1:]
parsed = urllib.parse.urlsplit(url)
if parsed.scheme != 'https' or not parsed.hostname or parsed.username or parsed.query or parsed.fragment:
    raise SystemExit('Use an HTTPS ingest URL without credentials or query parameters.')
path = Path(token_path)
if path.stat().st_mode & 0o077:
    raise SystemExit('Token file must be private (chmod 600).')
token = path.read_text().strip()
if not token.startswith('ding_service_') or any(c.isspace() for c in token):
    raise SystemExit('Expected one Ding producer token.')

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

client = urllib.request.build_opener(NoRedirect)
run = secrets.token_hex(12)
fd = os.open(ledger_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as ledger:
    def record(value):
        ledger.write(json.dumps(value) + '\n')
        ledger.flush()
        os.fsync(ledger.fileno())

    def send(key, body, expected, authenticated=True, media='application/json'):
        headers = {'Content-Type': media, 'User-Agent': 'ding-push-qualification/1.0'}
        if authenticated:
            headers['Authorization'] = 'Bearer ' + token
        if key is not None:
            headers['Idempotency-Key'] = key
        record({'stage': 'offered', 'key': key, 'body': body, 'expected': expected})
        req = urllib.request.Request(url, data=body.encode(), headers=headers)
        try:
            response = client.open(req, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data = json.loads(response.read(65536))
            record({'stage': 'response', 'key': key, 'status': response.status, 'data': data})
            if response.status != expected:
                raise SystemExit(f'Expected HTTP {expected}, received {response.status}; retain ledger for reconciliation.')
        time.sleep(0.15)
        return data.get('data', {})

    body = '{"repo":"qualification-0","failed":true}'
    send(run, body, 401, authenticated=False)
    send(None, body, 400)
    send(run, body, 415, media='text/plain')
    send(run, '{', 400)
    send(run, '{"failed":true}', 400)
    first = send(run, body, 202)
    replay = send(run, body, 202)
    assert replay['duplicate'] and replay['acceptanceId'] == first['acceptanceId']
    send(run, '{"repo":"qualification-0","failed":false}', 409)
    for repo in range(1, 18):
        send(f'{run}-{repo}', json.dumps({'repo': f'qualification-{repo}', 'failed': True}), 202)
    send(run + '-recovery', '{"repo":"qualification-0","failed":false}', 202)
print(json.dumps({'passed': True, 'uniqueAccepted': 19, 'exactReplays': 1,
                  'expectedOpenGroups': 17, 'ledger': ledger_path}))
