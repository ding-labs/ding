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
  assert.equal((await start(c)).status,200);
  assert.equal((await start(c)).status,409);
  assert.equal((await start({...c,mode:'measure'})).status,409);
  const b=await mf.getR2Bucket('EVIDENCE');assert.ok(await b.get(ROOT+'active.json'));assert.equal(await b.get('active.json'),null);
  assert.equal((await mf.dispatchFetch('https://test/admin/object?key=other/private',{headers})).status,404);
  assert.equal((await mf.dispatchFetch('https://test/admin/stop',{method:'POST',headers})).status,200);
  const status=await (await mf.dispatchFetch('https://test/admin/status',{headers})).json();assert.equal(status.stop.reason,'operator_stop');
 }finally{await mf.dispose();}
});
