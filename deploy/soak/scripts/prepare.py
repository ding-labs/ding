#!/usr/bin/env python3
"""Create scoped observer credentials once. Does not start the soak."""
import json
import secrets
import urllib.request
from datetime import datetime, timezone
from common import PRIVATE, ORIGIN, Owner, save, load

PROJECT = '8803ede4-af5e-4fe7-8176-9788c9c49319'
ENVIRONMENT = '68ccf6c6-4c03-43cb-8d28-7d37a5ed42d8'
SERVICE = 'ac520781-0a29-45bc-9601-3adea5de338f'
if (PRIVATE / 'active.json').exists():
    raise SystemExit('Existing preparation found; inspect it instead of creating duplicates.')
run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ-') + secrets.token_hex(8)
state = {'runId': run_id, 'status': 'setup', 'projectId': PROJECT, 'environmentId': ENVIRONMENT,
         'serviceId': SERVICE, 'accountId': 'b5a19010c0fa72255f749f63b2ec0962',
         'workerURL': 'https://ding-cloud-soak.zuchka.workers.dev',
         'applicationRevision': 'c5d9213eef4ad8049ef90d4d631e14eb6b419fc1',
         'createdAt': datetime.now(timezone.utc).isoformat()}
save(PRIVATE/'active.json', state)
save(PRIVATE/'secrets.json', {'admin': secrets.token_urlsafe(32)})
owner = Owner()
baseline = {path: owner.call(path)['data'] for path in ['/v1/watches', '/v1/watches/freedesign-http', '/v1/cloud/usage', '/v1/doctor']}
save(PRIVATE/'baseline.json', baseline)
created = owner.call('/v1/services', {'name': 'soak-inspect-' + run_id, 'scopes': ['inspect'], 'watches': [], 'days': 7})['data']
s = load('secrets.json')
s['ding'] = created['token']
save(PRIVATE/'secrets.json', s)
state['dingServiceId'] = created['id']
save(PRIVATE/'active.json', state)
headers = {'Authorization': 'Bearer ' + s['ding']}
with urllib.request.urlopen(urllib.request.Request(ORIGIN+'/v1/status',headers=headers)) as response:
    assert response.status == 200
try:
    urllib.request.urlopen(urllib.request.Request(ORIGIN+'/v1/apply',data=b'{}',headers={**headers,'Content-Type':'application/json'}))
except urllib.error.HTTPError as e:
    assert e.code == 403
else:
    raise AssertionError('Inspect token accepted a mutation')
print('Prepared private run state and Ding token; inspect read/write-denial verified. Run metrics.py after saving the Railway project token file.')
