#!/usr/bin/env python3
"""Validate Railway's fixed query with a production project token, never a personal token."""
import datetime
import json
import os
from pathlib import Path
import subprocess
import urllib.request
from common import PRIVATE, load, save

path=PRIVATE/'railway-project-token.txt'
if not path.exists():raise SystemExit('Waiting for the production project token file; no broad credential fallback.')
os.chmod(path,0o600)
token=path.read_text().strip()
if not token or len(token)>4096 or any(c.isspace() for c in token):raise SystemExit('Invalid token file')
script=Path(__file__).resolve().parents[1]/'worker/src/index.mjs'
query=subprocess.check_output(['node','--input-type=module','-e',"import {METRICS_QUERY} from '"+script.as_uri()+"'; console.log(METRICS_QUERY)"],text=True).strip()
now=datetime.datetime.now(datetime.timezone.utc)
body={'query':query,'variables':{'start':(now-datetime.timedelta(minutes=30)).isoformat(),'end':now.isoformat()}}
req=urllib.request.Request('https://backboard.railway.com/graphql/v2',data=json.dumps(body).encode(),headers={'User-Agent':'ding-soak-operator/1.0','Content-Type':'application/json','Project-Access-Token':token})
with urllib.request.urlopen(req,timeout=30) as r:result=json.load(r)
if result.get('errors'):raise SystemExit('Scoped metrics query rejected; no successful metrics evidence recorded.')
series=result.get('data',{}).get('metrics',[])+result.get('data',{}).get('disk',[])
required={'CPU_USAGE','MEMORY_USAGE_GB','DISK_USAGE_GB','NETWORK_RX_GB','NETWORK_TX_GB'}
if not required.issubset({s['measurement'] for s in series if s['values']}):raise SystemExit('Missing Railway series; inspect API schema and project-token scope.')
s=load('secrets.json');s['railway']=token;save(PRIVATE/'secrets.json',s)
save(PRIVATE/'railway-metrics-preflight.json',result)
p=load('preflight.json') if (PRIVATE/'preflight.json').exists() else {}
p['metrics']={'verdict':'pass','at':now.isoformat(),'series':sorted(required)};save(PRIVATE/'preflight.json',p)
print('PASS: production-scoped metrics returned CPU, memory, disk and network. Upload RAILWAY_TOKEN as a Worker secret before warmup.')
