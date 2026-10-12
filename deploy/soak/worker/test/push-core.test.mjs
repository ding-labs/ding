import test from 'node:test';
import assert from 'node:assert/strict';
import {validateStart,offer,report,receiptMatches,entitiesMatch,storagePressure,MINUTE,HOURS72} from '../src/push-core.mjs';
const now=Date.now(), epoch=Math.ceil(now/MINUTE)*MINUTE;
const c=validateStart({runId:'push-0123456789abcdef',watchId:'push-soak-01234567',mode:'measure',epoch,applicationRevision:'a'.repeat(40),observerRevision:'b'.repeat(40)},now);
test('measurement lasts 72 hours and refuses backdating',()=>{
 assert.equal(c.endsAt-c.epoch,HOURS72);
 assert.throws(()=>validateStart({...c,epoch:epoch-2*MINUTE},now));
 assert.throws(()=>validateStart({...c,runId:'../private'},now));
});
test('18 stable independent groups, deterministic retry bytes, final recovery',()=>{
 const first=offer(c,epoch); assert.equal(JSON.parse(first.body).events.length,18);
 assert.deepEqual(first,offer(c,epoch));
 assert.notEqual(first.body,offer(c,epoch+MINUTE).body);
 assert.equal(offer(c,c.endsAt-MINUTE).open,0);
 assert.equal(new Set(JSON.parse(first.body).events.map(e=>e.repo)).size,18);
});
test('receipt replay must preserve identity and observation range',()=>{
 const a={data:{acceptanceId:'one',first:1,last:18}};
 assert.ok(receiptMatches(a,{data:{...a.data,duplicate:true}}));
 assert.equal(receiptMatches(a,{data:{...a.data,duplicate:false}}),false);
 assert.equal(receiptMatches(a,{data:{...a.data,duplicate:true,last:19}}),false);
});
test('elapsed time alone cannot pass: missing and rejected samples remain failures',()=>{
 const samples=Array.from({length:4320},(_,i)=>({scheduledAt:epoch+i*MINUTE,pass:true,accept:{status:202}}));
 assert.equal(report(c,samples,c.endsAt).verdict,'pass');
 assert.equal(report(c,samples.slice(1),c.endsAt).missing,1);
 samples[5].pass=false; assert.equal(report(c,samples,c.endsAt).failed,1);
 assert.equal(report(c,[],epoch).verdict,'running');
});

test('wrong group identities cannot pass by matching only the incident count',()=>{
 const offered=offer(c,epoch),entities=JSON.parse(offered.body).events.map(e=>({key:JSON.stringify([{field:'repo',present:true,value:e.repo}]),open:e.failed}));
 assert.ok(entitiesMatch(entities,offered));entities[0].key=entities[1].key;assert.equal(entitiesMatch(entities,offered),false);
});
test('storage safety covers workspace allocation and the one GiB Railway volume',()=>{
 assert.equal(storagePressure({limits:{maxBytes:100},usage:{liveBytes:85}},{}),true);
 assert.equal(storagePressure({}, {disk:[{measurement:'DISK_USAGE_GB',values:[{value:0.9}]}]}),true);
 assert.equal(storagePressure({limits:{maxBytes:100},usage:{liveBytes:2}},{}),false);
});
