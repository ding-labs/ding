import test from 'node:test';
import assert from 'node:assert/strict';
import {report} from '../src/report.mjs';
import {MINUTE,HOUR} from '../src/core.mjs';
const epoch=Date.now()-72*HOUR;
const config={runId:'test',mode:'measure',epoch,endsAt:epoch+72*HOUR};
test('elapsed time with no evidence never passes',()=>{
 const r=report({config},config.endsAt+MINUTE);assert.equal(r.ready,false);assert.notEqual(r.verdict,'pass');assert.equal(r.gates.evidence.verdict,'fail');assert.equal(r.gates.correctness.verdict,'inconclusive');
});
test('malformed maintenance cannot excuse an outage retroactively',()=>{
 const at=epoch+MINUTE,r=report({config,samples:[{scheduledAt:at,startedAt:at,health:{ok:true,data:{status:503}}}],maintenance:[{start:epoch,end:epoch+HOUR,declaredAt:at}]});
 assert.equal(r.gates.availability.verdict,'fail');
});
test('missing firing evidence is a correctness failure',()=>{
 const r=report({config,fixtures:[{id:'one',kind:'status',phase:1,at:epoch+MINUTE}]},config.endsAt+MINUTE);
 assert.equal(r.gates.correctness.verdict,'fail');assert.equal(r.gates.correctness.details.mismatches[0].type,'firing');
});
