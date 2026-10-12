#!/usr/bin/env python3
"""Read loopback metrics and process/storage sizes over existing Railway SSH."""
import datetime
import json
import shlex
import subprocess
from common import PRIVATE, save, load

key=PRIVATE.parent/'railway/operator_ed25519'
base=['ssh','-o','BatchMode=yes','-i',str(key),'cloud.ding.ing@ssh.railway.com']
def remote(command):return subprocess.check_output(base+[command],text=True,timeout=40)
config=json.loads(remote('cat /run/ding/config.json'))
active=config['dataDir']
if not active.startswith('/data/') or '..' in active:raise SystemExit('Unexpected data directory')
metrics=remote('wget -qO- http://127.0.0.1:8788/metrics')
process=remote('cat /proc/1/status')
sizes=remote('du -sk '+shlex.quote(active)+'; df -k /data')
now=datetime.datetime.now(datetime.timezone.utc)
record={'at':now.isoformat(),'activeDirectory':active,'metrics':metrics,'processStatus':process,'storage':sizes}
path=PRIVATE/'checkpoints'/(now.strftime('%Y%m%dT%H%M%SZ')+'.json')
save(path,record)
assert 'ding_cloud_failed_workers 0' in metrics and 'ding_cloud_pending_deliveries 0' in metrics
p=load('preflight.json') if (PRIVATE/'preflight.json').exists() else {}
p['runtimeCheckpoint']={'verdict':'pass','path':str(path),'at':record['at'],'activeDirectory':active}
save(PRIVATE/'preflight.json',p)
print('PASS: active directory resolved; loopback metrics, process RSS and storage saved privately.')
