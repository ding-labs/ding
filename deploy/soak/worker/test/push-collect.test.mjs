import test from 'node:test';
import assert from 'node:assert/strict';
import {collectPush} from '../src/push-collect.mjs';
import {MINUTE,ROOT,offer} from '../src/push-core.mjs';
class Bucket {
 data=new Map();fail='';
 async get(k){const value=this.data.get(k);return value?{json:async()=>structuredClone(value)}:null;}
 async put(k,v,options){if(k.includes(this.fail)&&this.fail)throw Error('disk');if(options?.onlyIf&&this.data.has(k))return null;this.data.set(k,JSON.parse(v));return {etag:'yes'};}
}
const minute=Math.floor(Date.now()/MINUTE/5)*5*MINUTE+MINUTE;
const config={runId:'push-0123456789abcdef',watchId:'push-soak-01234567',mode:'warmup',epoch:minute-MINUTE,endsAt:minute+10*MINUTE};
test('offer precedes delivery, duplicate Cron does not send twice, receipts and state reconcile',async(t)=>{
 const b=new Bucket();b.data.set(ROOT+'active.json',config);let pushes=0;
 const expected=JSON.parse(offer(config,minute).body).events;
 t.mock.method(globalThis,'fetch',async(url,options)=>{
  if(url.endsWith('/healthz'))return new Response('ok');
  if(url.includes('/ingest/')){assert.ok(b.data.has(ROOT+`${config.runId}/warmup/offers/${minute}.json`));pushes++;return Response.json({data:{acceptanceId:'one',first:1,last:18,duplicate:pushes>1}} ,{status:202});}
  return Response.json({data:{watch:{status:'running',plan:{revision:'a'}},entities:expected.map(e=>({key:JSON.stringify([{field:'repo',present:true,value:e.repo}]),open:e.failed,sourceUnhealthy:false})),entitiesMore:false}});
 });
 await collectPush({scheduledTime:minute},{EVIDENCE:b});await collectPush({scheduledTime:minute},{EVIDENCE:b});
 assert.equal(pushes,2);assert.equal(b.data.get(ROOT+`${config.runId}/warmup/latest.json`).pass,true);
});
test('failed evidence persistence prevents sending and leaves visible missing sample',async(t)=>{
 const b=new Bucket();b.data.set(ROOT+'active.json',config);b.fail='/offers/';let requests=0;t.mock.method(globalThis,'fetch',()=>{requests++;});
 await assert.rejects(collectPush({scheduledTime:minute},{EVIDENCE:b}));assert.equal(requests,0);
 assert.equal(b.data.has(ROOT+`${config.runId}/warmup/samples/${minute}.json`),false);
});
test('deadline and durable stop prevent further producer traffic',async(t)=>{
 const b=new Bucket();b.data.set(ROOT+'active.json',{...config,endsAt:Date.now()-1});let requests=0;t.mock.method(globalThis,'fetch',()=>{requests++;});
 await collectPush({scheduledTime:minute},{EVIDENCE:b});b.data.set(ROOT+'active.json',config);b.data.set(ROOT+config.runId+'/stop.json',{reason:'test'});
 await collectPush({scheduledTime:minute},{EVIDENCE:b});assert.equal(requests,0);
});
