#!/usr/bin/env python3
"""Run ONLY after Railway is deployed in maintenance. Export and verify locally."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import urllib.error
import urllib.request
from common import PRIVATE, ORIGIN, save, load

ROOT=Path(__file__).resolve().parents[3]
try:
    urllib.request.urlopen(ORIGIN+'/healthz',timeout=10)
    raise SystemExit('Refusing online backup: health is not maintenance 503')
except urllib.error.HTTPError as e:
    if e.code!=503:raise
now=datetime.datetime.now(datetime.timezone.utc)
name='soak-'+now.strftime('%Y%m%dT%H%M%SZ')
active=load('preflight.json')['runtimeCheckpoint']['activeDirectory']
if not active.startswith('/data/') or '..' in active:raise SystemExit('Invalid state path')
recipient='age18vkdsmsumlfj95l7fyty5d3hj3wpdzwazxvng4qu33pg289nfccs3dk89h'
remote='/data/backups/'+name+'.age'
command=shlex.join(['/sbin/su-exec','10001:10001','/ding-cloud','backup','--data-dir',active,'--output',remote,'--recipient',recipient])
subprocess.run(['ssh','-o','BatchMode=yes','-i',str(PRIVATE.parent/'railway/operator_ed25519'),'cloud.ding.ing@ssh.railway.com',command],check=True,capture_output=True,text=True)
archive=PRIVATE/(name+'.age')
subprocess.run(['npx','--yes','@railway/cli@5.64.2','volume','files','--volume','ding-data','download','/backups/'+name+'.age',str(archive)],cwd=ROOT,check=True,capture_output=True,text=True)
os.chmod(archive,0o600)
restored=PRIVATE/(name+'-restore')
subprocess.run(['go','run','./cmd/ding-cloud','restore','--data-dir',str(restored),'--input',str(archive),'--identity-file',str(PRIVATE.parent/'railway/backup.identity')],cwd=ROOT,check=True,capture_output=True,text=True)
subprocess.run(['go','run','./deploy/soak/verify',str(restored),str(PRIVATE.parent/'railway/data.key'),str(PRIVATE/'recovery-probe.json'),'freedesign-http'],cwd=ROOT,check=True)
p=load('preflight.json')
p['backupRestore']={'verdict':'pass','at':now.isoformat(),'archive':str(archive),'restored':str(restored),'bytes':archive.stat().st_size,'sha256':hashlib.sha256(archive.read_bytes()).hexdigest(),'activeDirectory':active}
save(PRIVATE/'preflight.json',p)
print('Encrypted archive downloaded and isolated restoration verified. Resume Railway serve mode.')
