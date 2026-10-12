import {boundedJSON} from './core.mjs';
import {put,METRICS_QUERY} from './index.mjs';
import {MINUTE,ROOT,offer,receiptMatches} from './push-core.mjs';
const ORIGIN='https://cloud.ding.ing';
export async function read(env,key) {const obj=await env.EVIDENCE.get(ROOT+key);return obj?await obj.json():null;}
export async function write(env,key,value) {return put(env.EVIDENCE,ROOT+key,value,{etagDoesNotMatch:'*'});}
async function call(path,token,body,key) {
 const at=Date.now();
 try {
  const r=await fetch(ORIGIN+path,{method:body===undefined?'GET':'POST',headers:{Authorization:`Bearer ${token}`,'Content-Type':'application/json',...(key?{'Idempotency-Key':key}:{})},body,redirect:'manual',signal:AbortSignal.timeout(10000)});
  return {status:r.status,latencyMs:Date.now()-at,response:await boundedJSON(r,512*1024)};
 } catch { return {status:0,latencyMs:Date.now()-at,error:'request_unconfirmed'}; }
}
export async function collectPush(controller,env) {
 const now=Date.now(), minute=Math.floor(controller.scheduledTime/MINUTE)*MINUTE;
 await put(env.EVIDENCE,ROOT+'last-cron.json',{scheduledAt:minute,at:now});
 const c=await read(env,'active.json');
 if(!c || now<c.epoch || now>=c.endsAt || minute<c.epoch || minute>=c.endsAt || await read(env,c.runId+'/stop.json')) return;
 const base=`${c.runId}/${c.mode}`, id=String(minute);
 if(!await write(env,`${base}/claims/${id}.json`,{at:now}))return;
 const offered=offer(c,minute),sample={scheduledAt:minute,at:now,offered,attempts:[]};
 // The immutable offer is durable before any authenticated outbound request.
 await write(env,`${base}/offers/${id}.json`,offered);
 for(let attempt=0;attempt<3;attempt++) {
  sample.accept=await call('/v1/ingest/'+c.watchId,env.PRODUCER_TOKEN,offered.body,offered.key);
  sample.attempts.push(sample.accept);
  if(![0,429,503].includes(sample.accept.status))break;
  await new Promise(resolve=>setTimeout(resolve,2000));
 }
 if(sample.accept.status===202) {
  sample.replay=await call('/v1/ingest/'+c.watchId,env.PRODUCER_TOKEN,offered.body,offered.key);
  if(minute%(60*MINUTE)===0)sample.conflict=await call('/v1/ingest/'+c.watchId,env.PRODUCER_TOKEN,offered.body+' ',offered.key);
 }
 sample.state=await call('/v1/watches/'+c.watchId,env.INSPECT_TOKEN);
 const data=sample.state.response?.data;
 if(data)sample.state.response={data:{watch:{status:data.watch.status,generation:data.watch.generation,revision:data.watch.plan.revision,lastInputAt:data.watch.lastInputAt},entities:data.entities,entitiesMore:data.entitiesMore}};
 const entities=data?.entities,receipt=sample.accept.response?.data;
 sample.pass=sample.accept.status===202 && receipt?.last-receipt?.first+1===18 && sample.replay?.status===202 && receiptMatches(sample.accept.response,sample.replay?.response)
  && (!sample.conflict || sample.conflict.status===409) && sample.state.status===200 && entities?.length===18 && !data.entitiesMore
  && entities.every(e=>!e.sourceUnhealthy) && entities.filter(e=>e.open).length===offered.open;
 try {const r=await fetch(ORIGIN+'/healthz',{redirect:'manual',signal:AbortSignal.timeout(10000)});sample.health=r.status;await r.body?.cancel();}catch{sample.health=0;}
 sample.pass=sample.pass && sample.health===200;
 if(minute%(5*MINUTE)===0) {
  sample.usage=await call('/v1/cloud/usage',env.INSPECT_TOKEN);
  sample.doctor=await call('/v1/doctor',env.INSPECT_TOKEN);
  try {
   const r=await fetch('https://backboard.railway.com/graphql/v2',{method:'POST',headers:{'Content-Type':'application/json','User-Agent':'ding-push-soak/1.0','Project-Access-Token':env.RAILWAY_TOKEN},body:JSON.stringify({query:METRICS_QUERY,variables:{start:new Date(now-5*MINUTE).toISOString(),end:new Date(now).toISOString()}}),redirect:'manual',signal:AbortSignal.timeout(10000)});
   sample.railway={status:r.status,response:await boundedJSON(r)};
  }catch{sample.railway={status:0,error:'metrics_unavailable'};}
  sample.telemetryPass=sample.usage.status===200 && sample.doctor.status===200 && sample.railway.status===200 && !sample.railway.response?.errors && !!sample.railway.response?.data?.metrics?.length;
  sample.pass=sample.pass && sample.telemetryPass;
 }
 const previous=await read(env,base+'/latest.json');
 sample.failureStreak=sample.pass?0:(previous?.failureStreak||0)+1;
 sample.finishedAt=Date.now();
 await write(env,`${base}/samples/${id}.json`,sample);
 await put(env.EVIDENCE,ROOT+base+'/latest.json',sample);
 if(sample.failureStreak>=3)await write(env,c.runId+'/stop.json',{at:Date.now(),reason:'three_failed_samples'});
 console.log(JSON.stringify({kind:'push_soak',minute,pass:sample.pass,status:sample.accept.status}));
}
