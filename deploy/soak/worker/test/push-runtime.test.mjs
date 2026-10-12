import test from 'node:test';
import assert from 'node:assert/strict';
import {Miniflare,convertV4MiniflareOptions} from 'miniflare';
import {readFile} from 'node:fs/promises';
import {MINUTE,ROOT} from '../src/push-core.mjs';
test('Workers runtime enforces authorization, one-time start, warmup gate and independent R2 prefix',async()=>{
 const mf=new Miniflare(convertV4MiniflareOptions({modules:true,script:await readFile(new URL('../dist-push/push-entry.js',import.meta.url),'utf8'),compatibilityDate:'2026-10-12',r2Buckets:['EVIDENCE'],bindings:{ADMIN_TOKEN:'admin'}}));
 try {
  const headers={Authorization:'Bearer admin','Content-Type':'application/json'};
  assert.equal((await mf.dispatchFetch('https://test/admin/status')).status,404);
  const epoch=Math.ceil(Date.now()/MINUTE)*MINUTE,c={runId:'push-0123456789abcdef',watchId:'push-soak-20261012',mode:'warmup',epoch,applicationRevision:'a'.repeat(40),observerRevision:'b'.repeat(40)};
  const start=cfg=>mf.dispatchFetch('https://test/admin/start',{method:'PUT',headers,body:JSON.stringify(cfg)});
  assert.equal((await start({...c,mode:'measure'})).status,409);
  const race=await Promise.all([start(c),start({...c,runId:'push-1111111111111111'})]);
  assert.deepEqual(race.map(r=>r.status).sort(),[200,409]);
  const winner=await (await mf.dispatchFetch('https://test/admin/status',{headers})).json();
  c.runId=winner.config.runId;
  assert.equal((await start(c)).status,409);
  assert.equal((await start({...c,mode:'measure'})).status,409);
  const b=await mf.getR2Bucket('EVIDENCE');assert.ok(await b.get(ROOT+'active.json'));assert.equal(await b.get('active.json'),null);
  assert.equal((await mf.dispatchFetch('https://test/admin/object?key=other/private',{headers})).status,404);
  assert.equal((await mf.dispatchFetch('https://test/admin/stop',{method:'POST',headers})).status,200);
  const status=await (await mf.dispatchFetch('https://test/admin/status',{headers})).json();assert.equal(status.stop.reason,'operator_stop');
 }finally{await mf.dispose();}
});
test('real scheduled push uses stable bytes and keys, reconciles lost acknowledgement, and persists R2 evidence',async()=>{
 let calls=0,firstBody,firstKey;const now=Date.now(),minute=Math.floor(now/MINUTE/5)*5*MINUTE+MINUTE;
 const config={runId:'push-fedcba9876543210',watchId:'push-soak-20261012',mode:'warmup',epoch:minute-MINUTE,endsAt:now+10*MINUTE};
 const {offer}=await import('../src/push-core.mjs');
 const events=JSON.parse(offer(config,minute).body).events;
 const mf=new Miniflare(convertV4MiniflareOptions({modules:true,script:await readFile(new URL('../dist-push/push-entry.js',import.meta.url),'utf8'),compatibilityDate:'2026-10-12',r2Buckets:['EVIDENCE'],bindings:{ADMIN_TOKEN:'admin',PRODUCER_TOKEN:'producer',INSPECT_TOKEN:'inspect'},outboundService:async(request)=>{
  const path=new URL(request.url).pathname;
  if(path==='/healthz')return new Response('healthy');
  if(path.includes('/ingest/')) {
   const body=await request.text(),key=request.headers.get('Idempotency-Key');assert.equal(request.headers.get('Authorization'),'Bearer producer');
   if(!calls){firstBody=body;firstKey=key;}else{assert.equal(body,firstBody);assert.equal(key,firstKey);}
   calls++;
   if(calls===1)return Response.json({error:{code:'lost_ack'}},{status:503});
   return Response.json({data:{acceptanceId:'durable-id',first:1,last:18,duplicate:true}},{status:202});
  }
  assert.equal(request.headers.get('Authorization'),'Bearer inspect');
  return Response.json({data:{watch:{status:'running',plan:{revision:'a'}},entities:events.map(e=>({key:JSON.stringify([{field:'repo',present:true,value:e.repo}]),open:e.failed,sourceUnhealthy:false})),entitiesMore:false}});
 }}));
 try {
  const b=await mf.getR2Bucket('EVIDENCE');await b.put(ROOT+'active.json',JSON.stringify(config));
  const worker=await mf.getWorker();await worker.scheduled({scheduledTime:minute,cron:'* * * * *'});
  const sample=await (await b.get(ROOT+config.runId+'/warmup/latest.json')).json();
  assert.equal(calls,3);assert.equal(sample.pass,true);assert.equal(sample.attempts.length,2);assert.equal(sample.attempts[0].status,503);
  await worker.scheduled({scheduledTime:minute,cron:'* * * * *'});assert.equal(calls,3);
 }finally{await mf.dispose();}
});
