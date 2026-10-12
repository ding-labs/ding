import test from 'node:test';
import assert from 'node:assert/strict';
import {Miniflare,convertV4MiniflareOptions} from 'miniflare';
import {readFile} from 'node:fs/promises';
import {phase,fixture,expectedEvents,MINUTE,HOUR,quiet,boundedJSON,validateConfig} from '../src/core.mjs';
import {collect,collectEvents} from '../src/index.mjs';
const epoch=Date.now(), runId='test-0123456789abcdef';
const config={runId,mode:'warmup',epoch,endsAt:epoch+3*HOUR,applicationRevision:'a'.repeat(40),observerRevision:'b'.repeat(40)};
test('fixtures cover a cycle and become healthy at the hard deadline',()=>{
  for(let i=0;i<6;i++) assert.equal(phase(config,epoch+i*15*MINUTE),i);
  assert.equal(phase(config,config.endsAt),0);
  assert.equal(phase({...config,mode:'stopped'},epoch+30*MINUTE),0);
  assert.equal(fixture('status',2).status,500);
  assert.equal(fixture('status',0).body.length,140*1024);
  assert.equal(fixture('status',4).delay,7000);
  assert.throws(()=>JSON.parse(fixture('projection',2).body));
  assert.ok(fixture('projection',4).body.length>65536);
});
test('unknown holds the incident; 400 to 500 never opens twice',()=>{
  for(const kind of ['status','projection']) {
    const requests=Array.from({length:6},(_,phase)=>({id:String(phase),at:epoch+phase*15*MINUTE,kind,phase}));
    const types=expectedEvents(requests.flatMap(r=>[r,{...r,id:r.id+'b',at:r.at+MINUTE}])).map(e=>e.type);
    assert.deepEqual(types,kind==='status'?['firing','recovered','source_error','source_recovered']:['firing','source_error','source_recovered','recovered','source_error','source_recovered']);
  }
});
test('quiet window boundaries and configuration prevent backdated starts',()=>{
  const m={...config,mode:'measure',endsAt:epoch+72*HOUR};
  assert.equal(quiet(m,epoch+12*HOUR),true);assert.equal(quiet(m,epoch+18*HOUR),false);
  assert.throws(()=>validateConfig({...m,epoch:epoch-5*MINUTE},epoch));
  assert.throws(()=>validateConfig({...m,endsAt:epoch+1},epoch));
});
test('bounded decoding rejects oversized chunks',async()=>{
  await assert.rejects(boundedJSON(new Response('x'.repeat(65537)),65536),/response_too_large/);
});
class Bucket {
  data=new Map(); writes=[]; fail='';
  async get(key) {const v=this.data.get(key);return v?{etag:String(v.etag),json:async()=>structuredClone(v.value)}:null;}
  async put(key,value,options={}) {
    if(this.fail && key.includes(this.fail))throw Error('storage_unavailable');
    const existing=this.data.get(key),cond=options.onlyIf;
    if(cond?.etagDoesNotMatch==='*'&&existing)return null;
    if(cond?.etagMatches&&cond.etagMatches!==String(existing?.etag))return null;
    this.data.set(key,{value:JSON.parse(value),etag:(existing?.etag||0)+1});this.writes.push(key);return {etag:'ok'};
  }
}
test('cursor is not advanced if evidence cannot be stored',async(t)=>{
  const bucket=new Bucket();bucket.fail='/events/';
  t.mock.method(globalThis,'fetch',async(url)=>Response.json({data:String(url).includes('/observations')?{observations:[],more:false,after:0}:{events:[{id:'e',watchId:'soak-status'}],cursor:'new',more:false}}));
  await assert.rejects(collectEvents({EVIDENCE:bucket},'root'));
  assert.equal(bucket.data.has('root/cursor.json'),false);
});
test('retried scheduled invocation cannot duplicate a sample; failed write leaves a gap',async(t)=>{
  const bucket=new Bucket();const now=Date.now();
  const c={...config,mode:'measure',epoch:now-13*HOUR,endsAt:now+59*HOUR};
  await bucket.put('active.json',JSON.stringify(c));
  t.mock.method(globalThis,'fetch',async()=>new Response('ok'));
  const controller={scheduledTime:Math.floor(now/MINUTE/5)*5*MINUTE+MINUTE};
  await collect(controller,{EVIDENCE:bucket});await collect(controller,{EVIDENCE:bucket});
  assert.equal(bucket.writes.filter(k=>k.includes('/samples/')).length,1);
  assert.equal(bucket.data.get(`${runId}/measure/latest.json`).value.quiet,true);
  bucket.fail='/samples/';
  await assert.rejects(collect({scheduledTime:controller.scheduledTime+MINUTE},{EVIDENCE:bucket}));
  assert.equal(bucket.writes.filter(k=>k.includes('/samples/')).length,1);
});
test('real Workers runtime: private config, R2 conditional writes, bounded fixture and stop',async()=>{
  const mf=new Miniflare(convertV4MiniflareOptions({modules:true,script:await readFile(new URL('../dist/entry.js',import.meta.url),'utf8'),compatibilityDate:'2026-10-12',r2Buckets:['EVIDENCE'],bindings:{ADMIN_TOKEN:'test-admin'}}));
  try {
    assert.equal((await mf.dispatchFetch('https://test/admin/status')).status,404);
    const headers={Authorization:'Bearer test-admin','Content-Type':'application/json'};
    const now=Date.now();const cfg={...config,epoch:now,endsAt:now+3*HOUR};
    assert.equal((await mf.dispatchFetch('https://test/admin/config',{method:'PUT',headers,body:JSON.stringify(cfg)})).status,200);
    assert.equal((await mf.dispatchFetch('https://test/admin/config',{method:'PUT',headers,body:JSON.stringify(cfg)})).status,409);
    const bucket=await mf.getR2Bucket('EVIDENCE');
    const response=await mf.dispatchFetch(`https://test/fixture/${runId}/status`);
    assert.equal(response.status,200);assert.equal((await response.text()).length,140*1024);
    const logs=await bucket.list({prefix:runId+'/warmup/fixtures/'});assert.equal(logs.objects.length,1);
    assert.equal((await mf.dispatchFetch('https://test/admin/object?key=another-run/private',{headers})).status,404);
    const stop={...cfg,mode:'stopped',epoch:Date.now(),endsAt:Date.now()};stop.endsAt=stop.epoch;
    assert.equal((await mf.dispatchFetch('https://test/admin/config',{method:'PUT',headers,body:JSON.stringify(stop)})).status,200);
  }finally{await mf.dispose();}
});
test('scheduled collection uses edge-compatible requests and never follows redirects',async()=>{
  let redirect=false;
  const mf=new Miniflare(convertV4MiniflareOptions({modules:true,script:await readFile(new URL('../dist/entry.js',import.meta.url),'utf8'),compatibilityDate:'2026-10-12',r2Buckets:['EVIDENCE'],bindings:{ADMIN_TOKEN:'test-admin',DING_TOKEN:'inspect'},outboundService:async(request)=>{
    const p=new URL(request.url).pathname;
    if(p==='/healthz')return new Response('health',{status:redirect?302:200,headers:redirect?{Location:'https://elsewhere.test/'}:{}});
    if(p==='/v1/events')return Response.json({data:{events:[],cursor:'first',more:false}});
    return Response.json({data:{watch:{generation:1,plan:{revision:'abc'},status:'running',lastInputAt:new Date().toISOString(),nextAt:new Date(Date.now()+MINUTE).toISOString()},entities:[],deliveries:[]}});
  }}));
  try {
    const b=await mf.getR2Bucket('EVIDENCE');
    const c={...config,epoch:Date.now()-MINUTE};c.endsAt=c.epoch+3*HOUR;
    await b.put('active.json',JSON.stringify(c));
    const worker=await mf.getWorker();
    const time=Math.floor(Date.now()/MINUTE/5)*5*MINUTE+MINUTE;
    await worker.scheduled({scheduledTime:time,cron:'* * * * *'});
    let sample=await (await b.get(`${runId}/warmup/latest.json`)).json();
    assert.equal(sample.health.ok,true);assert.equal(sample.health.data.status,200);
    assert.equal(sample.ding['soak-status'].ok,true);assert.equal(sample.ding.events.ok,true);
    redirect=true;await worker.scheduled({scheduledTime:time+MINUTE,cron:'* * * * *'});
    sample=await (await b.get(`${runId}/warmup/latest.json`)).json();assert.equal(sample.health.data.status,302);
  }finally{await mf.dispose();}
});
