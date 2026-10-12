#!/usr/bin/env python3
"""Soak operator. No destructive production recovery; freedesign is preserved."""
import argparse
import json
import subprocess
import time
import urllib.parse
from pathlib import Path
from common import PRIVATE, Owner, observer, load, save

ROOT = Path(__file__).resolve().parents[1]

def export():
    status = observer('/admin/status')
    config = status['config']
    folder = PRIVATE/'evidence'/config['runId']/config['mode']
    save(folder/'config.json', config)
    save(folder/'maintenance.json', status.get('maintenance') or [])
    save(folder/'safety.json', status.get('safety'))
    prefix = config['runId']+'/'+config['mode']+'/'
    cursor = None
    while True:
        query = {'prefix': prefix}
        if cursor: query['cursor'] = cursor
        page = observer('/admin/list?' + urllib.parse.urlencode(query))
        for obj in page['objects']:
            relative = obj['key'][len(prefix):]
            if not relative.startswith(('samples/', 'events/', 'fixtures/')): continue
            path = folder/relative
            if path.exists(): continue
            save(path, observer('/admin/object?' + urllib.parse.urlencode({'key':obj['key']})))
        cursor = page['cursor']
        if not cursor: break
    for name in ['drills']:
        path = PRIVATE/(name+'.json')
        if path.exists(): save(folder/(name+'.json'), json.loads(path.read_text()))
    subprocess.run(['node', str(ROOT/'scripts/report.mjs'), str(folder)], check=True)
    return folder, json.loads((folder/'report.json').read_text())

def configure(mode):
    active=load()
    if mode=='warmup':
        preflight=load('preflight.json')
        cron=observer('/admin/status').get('lastCron')
        if not cron or time.time()*1000-cron['at']>90000:raise SystemExit('Wait for a real recent Cloudflare Cron invocation before warmup')
        required=['backupRestore','metrics','workerDeployed','quota','runtimeCheckpoint']
        if any(preflight.get(k,{}).get('verdict')!='pass' for k in required):
            raise SystemExit('Preflight incomplete: '+', '.join(k for k in required if preflight.get(k,{}).get('verdict')!='pass'))
    if mode=='measure':
        folder,result=export()
        if observer('/admin/status').get('safety'):raise SystemExit('Safety stop is active; preserve run and investigate')
        if not result['ready']:raise SystemExit('Dry-run gate did not pass. Inspect private report.json.')
        if (folder/'drills.json').exists():raise SystemExit('Unexpected measurement drills in warmup evidence')
    now=int(time.time()*1000)+(30000 if mode=='warmup' else 0)
    revision=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    config={k:active[k] for k in ['runId','applicationRevision']}
    config.update(mode=mode,epoch=now,endsAt=now+({'warmup':3,'measure':72,'stopped':0}[mode])*3600000,observerRevision=revision)
    observer('/admin/config',config,'PUT')
    active.update(status=mode,config=config)
    save(PRIVATE/'active.json',active)
    save(PRIVATE/('manifest-'+mode+'.json'),config)
    if mode=='measure':
        save(PRIVATE/'checkpoints.json',{'24h':now+24*3600000,'48h':now+48*3600000,'72h':config['endsAt'],
             'quietStart':now+12*3600000,'quietEnd':now+18*3600000,
             'note':'Declare exact restart/export windows before acting, in working hours 24–48. Total interruption <=10 minutes.'})
    print(json.dumps(config,indent=2))

def watches():
    active=load()
    owner=Owner()
    before=owner.call('/v1/watches/freedesign-http')['data']['watch']
    if before['plan']['revision']!=load('baseline.json')['/v1/watches/freedesign-http']['watch']['plan']['revision']:
        raise SystemExit('Real watch changed; reconcile before continuing')
    for kind in ['status','projection']:
        text=(ROOT/'manifests'/('soak-'+kind+'.yaml')).read_text().replace('FIXTURE_BASE',active['workerURL']+'/fixture/'+active['runId'])
        preview=owner.call('/v1/apply',{'manifest':text,'dryRun':True})['data']
        save(PRIVATE/('review-'+kind+'.json'),preview)
        applied=owner.call('/v1/apply',{'manifest':text,'review':preview['review']})
        save(PRIVATE/('applied-'+kind+'.json'),applied)
    print('Synthetic manifests reviewed and applied; freedesign revision preserved.')

def cleanup():
    cfg=observer('/admin/status')['config']
    if cfg['mode']!='measure' or time.time()*1000<cfg['endsAt']+6*60000:
        raise SystemExit('Wait until end plus one interval and verify healthy recovery first')
    owner=Owner()
    for ident in ['soak-status','soak-projection']:
        d=owner.call('/v1/watches/'+ident)['data']
        if d['watch']['status']=='deleted':continue
        if any(e['open'] or e['sourceUnhealthy'] for e in d['entities']):raise SystemExit(ident+' has not recovered; preserve evidence')
        if d.get('entitiesMore'):raise SystemExit('Unexpected extra entities')
        w=d['watch']
        owner.call('/v1/watches/'+ident+'/lifecycle',{'action':'delete','expected':w['plan']['revision'],'expectedGeneration':w['generation'],'cancelPending':True})
    export()
    configure('stopped')
    owner.call('/v1/services/'+load()['dingServiceId'],method='DELETE')
    print('Synthetic watches deleted and Ding inspection token revoked. Disable the Worker cron, revoke the Railway token and preserve/export evidence using the runbook.')

parser=argparse.ArgumentParser()
parser.add_argument('command',choices=['status','export','warmup','start','apply-watches','cleanup'])
args=parser.parse_args()
if args.command=='status':
    status=observer('/admin/status')
    print(json.dumps({'config':status.get('config'),'latestAt':status.get('latest',{}).get('startedAt') if status.get('latest') else None,'now':status['now']},indent=2))
elif args.command=='export':export()
elif args.command=='warmup':configure('warmup')
elif args.command=='start':configure('measure')
elif args.command=='apply-watches':watches()
elif args.command=='cleanup':cleanup()
