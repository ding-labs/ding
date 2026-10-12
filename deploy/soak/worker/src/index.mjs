import {MINUTE,HOUR,WATCHES,REVISION,phase,fixture,quiet,validateConfig,boundedJSON} from './core.mjs';
const ORIGIN='https://cloud.ding.ing';
const PROJECT='8803ede4-af5e-4fe7-8176-9788c9c49319';
const ENVIRONMENT='68ccf6c6-4c03-43cb-8d28-7d37a5ed42d8';
const SERVICE='ac520781-0a29-45bc-9601-3adea5de338f';
const VOLUME='c28bc5f6-db30-4266-aa36-148418a0b109';
export const METRICS_QUERY=`query($start:DateTime!,$end:DateTime!){metrics(projectId:"${PROJECT}",environmentId:"${ENVIRONMENT}",serviceId:"${SERVICE}",startDate:$start,endDate:$end,sampleRateSeconds:60,measurements:[CPU_USAGE,MEMORY_USAGE_GB,NETWORK_RX_GB,NETWORK_TX_GB]){measurement values{ts value}} disk:metrics(projectId:"${PROJECT}",environmentId:"${ENVIRONMENT}",volumeId:"${VOLUME}",startDate:$start,endDate:$end,sampleRateSeconds:60,measurements:[DISK_USAGE_GB]){measurement values{ts value}}}`;
const json=(value,status=200)=>Response.json(value,{status,headers:{'Cache-Control':'no-store'}});
export async function put(bucket,key,value,onlyIf) {
  return bucket.put(key,JSON.stringify(value),{httpMetadata:{contentType:'application/json'},...(onlyIf?{onlyIf}: {})});
}
async function read(bucket,key) {const o=await bucket.get(key);return o ? {etag:o.etag,value:await o.json()} : null;}
async function authorized(request,env) {
  if (!env.ADMIN_TOKEN) return false;
  const key=await crypto.subtle.importKey('raw',new TextEncoder().encode(env.ADMIN_TOKEN),{name:'HMAC',hash:'SHA-256'},false,['sign','verify']);
  const expected=await crypto.subtle.sign('HMAC',key,new TextEncoder().encode(`Bearer ${env.ADMIN_TOKEN}`));
  return crypto.subtle.verify('HMAC',key,expected,new TextEncoder().encode(request.headers.get('Authorization')||''));
}
async function ding(path,env) {
  const response=await fetch(ORIGIN+path,{headers:{Authorization:`Bearer ${env.DING_TOKEN}`},signal:AbortSignal.timeout(10000),redirect:'error'});
  if (!response.ok) {await response.body?.cancel();throw Error(`ding_http_${response.status}`);}
  const result=await boundedJSON(response);
  if (result.error || result.data === undefined) throw Error('ding_envelope');
  return result.data;
}
function errorCode(e) {return /^(ding_http_\d+|response_too_large|event_backlog|observation_backlog|railway_[a-z_]+)$/.test(e.message)?e.message:e.name==='TimeoutError'?'timeout':'collector_failed';}
async function captured(fn) {try{return {ok:true,data:await fn()};}catch(e){return {ok:false,error:errorCode(e)};}}
function eventMetadata(e) {return {id:e.id,sequence:e.sequence,watchId:e.watchId,revision:e.revision,type:e.type,at:e.at,evidence:e.evidence,fields:e.watchId.startsWith('soak-')?e.fields:undefined};}
export async function collectEvents(env,root) {
  const original=await read(env.EVIDENCE,root+'/cursor.json');
  let cursor=original?.value.cursor || '', count=0, more=false;
  for (let page=0;page<2;page++) {
    const data=await ding('/v1/events?limit=10'+(cursor?'&cursor='+encodeURIComponent(cursor):''),env);
    for(const e of data.events) {
      // Evidence endpoints are bounded and may paginate. Never advance past incomplete evidence.
      let after=0, observations=[], complete=false;
      for(let i=0;i<3;i++) {
        const p=await ding(`/v1/events/${encodeURIComponent(e.id)}/observations?after=${after}`,env);
        observations.push(...p.observations.map(o=>({sequence:o.sequence,acceptedAt:o.acceptedAt,health:o.health,detail:o.detail,
          fields:e.watchId.startsWith('soak-')?o.fields:undefined})));
        after=p.after;
        if(!p.more){complete=true;break;}
      }
      if(!complete) throw Error('observation_backlog');
      await put(env.EVIDENCE,`${root}/events/${e.id}.json`,{event:eventMetadata(e),observations},{etagDoesNotMatch:'*'});
      count++;
    }
    cursor=data.cursor;more=data.more;
    if(!more)break;
  }
  // CAS prevents overlapping minutes from rewinding the durable cursor.
  const committed=await put(env.EVIDENCE,root+'/cursor.json',{cursor,at:Date.now()},original?{etagMatches:original.etag}:{etagDoesNotMatch:'*'});
  if(!committed)throw Error('cursor_conflict');
  return {count,more};
}
export async function collect(controller,env) {
  const current=await read(env.EVIDENCE,'active.json');if(!current)return;
  const config=current.value,now=Date.now();
  if(config.mode==='stopped'||now<config.epoch||now>config.endsAt+10*MINUTE)return;
  const root=`${config.runId}/${config.mode}`;
  const minute=Math.floor(controller.scheduledTime/MINUTE)*MINUTE;
  // At-most-one attempt per minute. A crash leaves a visible missing sample; it cannot fabricate success.
  if(!await put(env.EVIDENCE,`${root}/claims/${minute}.json`,{at:now},{etagDoesNotMatch:'*'}))return;
  const previous=await read(env.EVIDENCE,root+'/latest.json');
  const maintenance=(await read(env.EVIDENCE,config.runId+'/maintenance.json'))?.value||[];
  const planned=maintenance.some(w=>now>=w.start&&now<w.end);
  const sample={version:REVISION,scheduledAt:minute,startedAt:now,quiet:quiet(config,now),planned};
  sample.health=await captured(async()=>{const at=Date.now();const r=await fetch(ORIGIN+'/healthz',{signal:AbortSignal.timeout(10000),redirect:'error'});await r.body?.cancel();return{status:r.status,latencyMs:Date.now()-at};});
  if(!sample.quiet) {
    const names=[...WATCHES,'events'];
    const results=await Promise.all([...WATCHES.map(id=>captured(async()=>{const d=await ding('/v1/watches/'+id,env);return {watch:{id,generation:d.watch.generation,revision:d.watch.plan.revision,status:d.watch.status,lastInputAt:d.watch.lastInputAt,nextAt:d.watch.nextAt},entities:d.entities,entitiesMore:d.entitiesMore,deliveries:d.deliveries};})),captured(()=>collectEvents(env,root))]);
    sample.ding=Object.fromEntries(names.map((n,i)=>[n,results[i]]));
  }
  if(minute%(5*MINUTE)===0) {
    sample.railway=await captured(async()=>{
      if(!env.RAILWAY_TOKEN)throw Error('railway_token_missing');
      const r=await fetch('https://backboard.railway.com/graphql/v2',{method:'POST',headers:{'User-Agent':'ding-soak-observer/1.0','Content-Type':'application/json','Project-Access-Token':env.RAILWAY_TOKEN},body:JSON.stringify({query:METRICS_QUERY,variables:{start:new Date(now-15*MINUTE).toISOString(),end:new Date(now).toISOString()}}),signal:AbortSignal.timeout(10000),redirect:'error'});
      if(!r.ok){await r.body?.cancel();throw Error('railway_http_error');}
      const d=await boundedJSON(r);if(d.errors||!d.data)throw Error('railway_query_error');
      if(!d.data.metrics?.length||!d.data.disk?.length||[...d.data.metrics,...d.data.disk].some(s=>!s.values?.length))throw Error('railway_missing_series');
      return d.data;
    });
    if(!sample.quiet) {
      sample.usage=await captured(()=>ding('/v1/cloud/usage',env));
      sample.doctor=await captured(()=>ding('/v1/doctor',env));
    }
  }
  sample.failedHealthStreak=!planned&&(!sample.health.ok||sample.health.data.status!==200)?(previous?.value.failedHealthStreak||0)+1:0;
  const overdue=Object.values(sample.ding||{}).some(v=>v.ok&&v.data.watch&&now-Date.parse(v.data.watch.nextAt)>5*MINUTE);
  const exhausted=sample.usage?.ok&&Object.entries(sample.usage.data.limits).some(([k,v])=>(sample.usage.data.usage[k]||0)>v*0.9);
  if(sample.failedHealthStreak>=3||(!planned&&overdue)||exhausted)
    await put(env.EVIDENCE,config.runId+'/safety.json',{at:now,reason:sample.failedHealthStreak>=3?'health_failures':overdue?'watch_overdue':'budget_low'},{etagDoesNotMatch:'*'});
  sample.finishedAt=Date.now();
  await put(env.EVIDENCE,`${root}/samples/${minute}.json`,sample,{etagDoesNotMatch:'*'});
  await put(env.EVIDENCE,`${root}/latest.json`,sample);
  console.log(JSON.stringify({kind:'soak_sample',runId:config.runId,scheduledAt:minute,health:sample.health.ok?sample.health.data.status:sample.health.error,quiet:sample.quiet}));
}
export default {
  async scheduled(controller,env) {await collect(controller,env);},
  async fetch(request,env) {
    const url=new URL(request.url),bucket=env.EVIDENCE;
    const current=await read(bucket,'active.json');
    const config=current?.value;
    const match=/^\/fixture\/([A-Za-z0-9-]+)\/(status|projection)$/.exec(url.pathname);
    if(match&&request.method==='GET'&&config&&match[1]===config.runId) {
      const at=Date.now(),safety=await read(bucket,config.runId+'/safety.json'),p=safety?0:phase(config,at),f=fixture(match[2],p),id=crypto.randomUUID();
      if(config.mode!=='stopped'&&at<=config.endsAt+10*MINUTE)
        await put(bucket,`${config.runId}/${config.mode}/fixtures/${at}-${id}.json`,{id,at,kind:match[2],phase:p,forcedHealthy:!!safety,intendedStatus:f.status,delayMs:f.delay,health:f.health,firing:f.firing},{etagDoesNotMatch:'*'});
      if(f.delay)await new Promise(resolve=>setTimeout(resolve,f.delay));
      return new Response(f.body,{status:f.status,headers:{'Content-Type':match[2]==='projection'?'application/json':'text/plain','Cache-Control':'no-store'}});
    }
    if(!url.pathname.startsWith('/admin/')||!await authorized(request,env))return json({error:'not_found'},404);
    if(url.pathname==='/admin/status'&&request.method==='GET')return json({config,safety:config?(await read(bucket,config.runId+'/safety.json'))?.value:null,maintenance:config?(await read(bucket,config.runId+'/maintenance.json'))?.value:[],latest:config?(await read(bucket,`${config.runId}/${config.mode}/latest.json`))?.value:null,now:Date.now()});
    if(url.pathname==='/admin/config'&&request.method==='PUT') {
      try {
        const next=validateConfig(await boundedJSON(request,8192),Date.now());
        if(config&&(next.runId!==config.runId||config.mode==='stopped'||config.mode==='measure'&&next.mode!=='stopped'||config.mode==='warmup'&&next.mode==='warmup'))return json({error:'invalid_transition'},409);
        await put(bucket,`${next.runId}/manifests/${next.mode}-${next.epoch}.json`,next,{etagDoesNotMatch:'*'});
        const ok=await put(bucket,'active.json',next,current?{etagMatches:current.etag}:{etagDoesNotMatch:'*'});
        return json({ok:!!ok},ok?200:409);
      }catch{return json({error:'invalid_config'},400);}
    }
    if(url.pathname==='/admin/maintenance'&&request.method==='POST'&&config?.mode==='measure') {
      try {
        const window=await boundedJSON(request,1024),now=Date.now();
        const prior=await read(bucket,config.runId+'/maintenance.json'),windows=prior?.value||[];
        if(!Number.isSafeInteger(window.start)||!Number.isSafeInteger(window.end)||window.start<now+30000||window.end<=window.start||window.end>config.endsAt||windows.reduce((n,w)=>n+w.end-w.start,window.end-window.start)>10*MINUTE)throw Error('invalid_window');
        const record={start:window.start,end:window.end,declaredAt:now};
        await put(bucket,config.runId+'/maintenance/'+now+'.json',record,{etagDoesNotMatch:'*'});
        const ok=await put(bucket,config.runId+'/maintenance.json',[...windows,record],prior?{etagMatches:prior.etag}:{etagDoesNotMatch:'*'});
        return json({ok:!!ok,window:record},ok?200:409);
      }catch{return json({error:'invalid_maintenance'},400);}
    }
    const prefix=url.searchParams.get('prefix')||'';
    if(url.pathname==='/admin/list'&&request.method==='GET'&&config&&prefix.startsWith(config.runId+'/')) {
      const list=await bucket.list({prefix,limit:100,cursor:url.searchParams.get('cursor')||undefined});
      return json({objects:list.objects.map(o=>({key:o.key,size:o.size})),cursor:list.truncated?list.cursor:null});
    }
    const key=url.searchParams.get('key')||'';
    if(url.pathname==='/admin/object'&&request.method==='GET'&&config&&key.startsWith(config.runId+'/')) {
      const o=await bucket.get(key);return o?new Response(o.body,{headers:{'Content-Type':'application/json','Cache-Control':'no-store'}}):json({error:'not_found'},404);
    }
    return json({error:'not_found'},404);
  }
};
