export const MINUTE = 60000;
export const HOURS72 = 72 * 60 * MINUTE;
export const ROOT = 'push-soak/';
export function validateStart(c, now) {
  if (!c || !/^push-[a-f0-9]{16}$/.test(c.runId) || !/^push-soak-[a-f0-9]{8}$/.test(c.watchId)) throw Error('invalid_identity');
  if (!['warmup', 'measure'].includes(c.mode)) throw Error('invalid_mode');
  if (!Number.isSafeInteger(c.epoch) || c.epoch % MINUTE || c.epoch < now || c.epoch > now + 2 * MINUTE) throw Error('invalid_epoch');
  if (!/^[a-f0-9]{40}$/.test(c.applicationRevision) || !/^[a-f0-9]{40}$/.test(c.observerRevision)) throw Error('missing_revision');
  return {runId:c.runId, watchId:c.watchId, mode:c.mode, epoch:c.epoch,
    endsAt:c.epoch + (c.mode === 'measure' ? HOURS72 : 10 * MINUTE),
    applicationRevision:c.applicationRevision, observerRevision:c.observerRevision};
}
export function offer(config, minute) {
  const index = Math.floor((minute - config.epoch) / MINUTE);
  // End with healthy inputs; each entity also has an independent staggered phase.
  const recovery = minute >= config.endsAt - MINUTE;
  const events = Array.from({length:18}, (_,i) => ({repo:`soak-${i}`, failed: !recovery && Math.floor((index + i) / 15) % 2 === 1}));
  return {key:`${config.runId}-${config.mode}-${index}`, body:JSON.stringify({events}),
    open:events.filter(e=>e.failed).length, outputs:events.length};
}
export function receiptMatches(first, replay) {
  const a=first?.data, b=replay?.data;
  return !!(a?.acceptanceId && b?.duplicate === true && a.acceptanceId === b.acceptanceId && a.first === b.first && a.last === b.last);
}
export function report(config, samples, now) {
  const expected = Math.min((config.endsAt-config.epoch)/MINUTE, Math.max(0, Math.floor((now-config.epoch)/MINUTE)));
  const byMinute = new Map(samples.map(s=>[s.scheduledAt,s]));
  let missing=0, failed=0, accepted=0;
  for(let i=0;i<expected;i++) {
    const s=byMinute.get(config.epoch+i*MINUTE);
    if(!s) {missing++;continue;}
    if(s.pass !== true) failed++;
    if(s.accept?.status===202) accepted++;
  }
  return {verdict:missing||failed?'fail':now<config.endsAt?'running':'pass',expected,recorded:byMinute.size,missing,failed,accepted};
}
