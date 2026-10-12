#!/usr/bin/env python3
"""Save project usage and provider billing-cycle estimates; never label forecasts as charges."""
import datetime
import json
import urllib.request
from common import PRIVATE, load, save

project=load()['projectId']
query='query{usage(projectId:"'+project+'",measurements:[CPU_USAGE,MEMORY_USAGE_GB,DISK_USAGE_GB,NETWORK_TX_GB]){measurement value} estimatedUsage(projectId:"'+project+'",measurements:[CPU_USAGE,MEMORY_USAGE_GB,DISK_USAGE_GB,NETWORK_TX_GB]){measurement estimatedValue}}'
req=urllib.request.Request('https://backboard.railway.com/graphql/v2',data=json.dumps({'query':query}).encode(),headers={'User-Agent':'ding-soak-operator/1.0','Content-Type':'application/json','Project-Access-Token':load('secrets.json')['railway']})
with urllib.request.urlopen(req,timeout=30) as r:result=json.load(r)
now=datetime.datetime.now(datetime.timezone.utc)
save(PRIVATE/'billing'/(now.strftime('%Y%m%dT%H%M%SZ')+'.json'),{'at':now.isoformat(),'provider':'railway','response':result,'note':'Usage and end-of-cycle estimates from provider; not an invoice or final cost. Reconcile with Railway and Cloudflare billing at completion.'})
if result.get('errors'):raise SystemExit('Provider did not expose all billing data; saved response privately. Billing remains unqualified.')
print('Project usage and end-of-cycle estimates saved privately. Final billed cost remains to be reconciled.')
