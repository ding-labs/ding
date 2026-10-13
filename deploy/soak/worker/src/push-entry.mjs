import {boundedJSON} from './core.mjs';
import {put} from './index.mjs';
import {ROOT,validateStart,report} from './push-core.mjs';
import {read,write,collectPush} from './push-collect.mjs';
const json=(v,status=200)=>Response.json(v,{status,headers:{'Cache-Control':'no-store'}});
async function authorized(request,env) {
 if(!env.ADMIN_TOKEN)return false;
 const encode=s=>new TextEncoder().encode(s);
 const key=await crypto.subtle.importKey('raw',encode(env.ADMIN_TOKEN),{name:'HMAC',hash:'SHA-256'},false,['sign','verify']);
 return crypto.subtle.verify('HMAC',key,await crypto.subtle.sign('HMAC',key,encode('Bearer '+env.ADMIN_TOKEN)),encode(request.headers.get('Authorization')||''));
}
async function samples(env,c) {
 const out=[];let cursor;
 do {
  const page=await env.EVIDENCE.list({prefix:ROOT+`${c.runId}/${c.mode}/samples/`,limit:1000,cursor});
  for(const obj of page.objects)out.push(await read(env,obj.key.slice(ROOT.length)));
  cursor=page.truncated?page.cursor:undefined;
 }while(cursor);
 return out;
}
export default {
 async scheduled(controller,env){await collectPush(controller,env);},
 async fetch(request,env) {
  const url=new URL(request.url);
  if(!await authorized(request,env))return json({error:'not_found'},404);
  if(url.pathname==='/admin/check-credentials'&&request.method==='GET') {
   const statuses={};
   for(const [role,token] of [['producer',env.PRODUCER_TOKEN],['inspect',env.INSPECT_TOKEN]]) {
    if(!token){statuses[role]=0;continue;}
    try {
     const r=await fetch('https://cloud.ding.ing/v1/status',{headers:{Authorization:`Bearer ${token}`},redirect:'manual',signal:AbortSignal.timeout(10000)});
     statuses[role]=r.status;await r.body?.cancel();
    }catch{statuses[role]=0;}
   }
   return json({statuses,pass:statuses.producer===403&&statuses.inspect===200});
  }
  const active=await env.EVIDENCE.get(ROOT+'active.json');
  const c=active?await active.json():null;
  if(url.pathname==='/admin/status'&&request.method==='GET')return json({config:c,lastCron:await read(env,'last-cron.json'),latest:c?await read(env,`${c.runId}/${c.mode}/latest.json`):null,stop:c?await read(env,c.runId+'/stop.json'):null,now:Date.now()});
  if(url.pathname==='/admin/start'&&request.method==='PUT') {
   try {
    const input=await boundedJSON(request,4096),next=validateStart(input,Date.now());
    if(input.previousRunId) {
     if(!c||input.previousRunId!==c.runId||next.runId===c.runId||next.mode!=='warmup'||!await read(env,c.runId+'/stop.json'))return json({error:'stopped_run_required'},409);
     next.previousRunId=c.runId;
    }else if(c) {
     if(c.mode!=='warmup'||next.mode!=='measure'||c.runId!==next.runId||c.watchId!==next.watchId||await read(env,c.runId+'/stop.json'))return json({error:'invalid_transition'},409);
     const warmup=await samples(env,c),gate=report(c,warmup,Date.now());
     if(gate.missing||gate.failed||gate.accepted<3||!warmup.some(s=>s.telemetryPass===true))return json({error:'warmup_not_ready',gate},409);
    }else if(next.mode!=='warmup')return json({error:'warmup_required'},409);
    // Archive the immutable configuration before publishing the active pointer.
    if(!await write(env,`${next.runId}/${next.mode}/manifest.json`,next))return json({error:'already_started'},409);
    const ok=await put(env.EVIDENCE,ROOT+'active.json',next,active?{etagMatches:active.etag}:{etagDoesNotMatch:'*'});
    return json({ok:!!ok,config:next},ok?200:409);
   }catch{return json({error:'invalid_start'},400);}
  }
  if(url.pathname==='/admin/stop'&&request.method==='POST'&&c){await write(env,c.runId+'/stop.json',{at:Date.now(),reason:'operator_stop'});return json({stopped:true});}
  if(url.pathname==='/admin/list'&&request.method==='GET'&&c) {
   const prefix=url.searchParams.get('prefix')||c.runId+'/';
   if(!prefix.startsWith(c.runId+'/'))return json({error:'not_found'},404);
   const p=await env.EVIDENCE.list({prefix:ROOT+prefix,limit:1000,cursor:url.searchParams.get('cursor')||undefined});
   return json({objects:p.objects.map(o=>({key:o.key.slice(ROOT.length),size:o.size})),cursor:p.truncated?p.cursor:null});
  }
  if(url.pathname==='/admin/object'&&request.method==='GET'&&c) {
   const key=url.searchParams.get('key')||'';
   if(!key.startsWith(c.runId+'/'))return json({error:'not_found'},404);
   const obj=await env.EVIDENCE.get(ROOT+key);return obj?new Response(obj.body,{headers:{'Content-Type':'application/json','Cache-Control':'no-store'}}):json({error:'not_found'},404);
  }
  return json({error:'not_found'},404);
 }
};
