export const MINUTE = 60000;
export const HOUR = 60 * MINUTE;
export const WATCHES = ['freedesign-http', 'soak-status', 'soak-projection'];
export function watchIds(config) {return ['freedesign-http',config.watchIds?.status||'soak-status',config.watchIds?.projection||'soak-projection'];}
export const REVISION = 'soak-v1';
export function phase(config, now) {
  if (!config || !['warmup', 'measure'].includes(config.mode) || now < config.epoch || now >= config.endsAt) return 0;
  return Math.floor((now - config.epoch) / (15 * MINUTE)) % 6;
}
export function fixture(kind, stage) {
  if (kind === 'status') return {status: [200, 400, 500, 200, 200, 200][stage], delay: stage === 4 ? 7000 : 0,
    body: stage === 4 ? 'delayed' : 'x'.repeat(140 * 1024), health: stage === 4 ? 'unknown' : 'ok', firing: stage === 1 || stage === 2};
  return {status: 200, delay: 0, body: stage === 2 ? '{broken' : stage === 4 ? JSON.stringify({value: 0, padding: 'x'.repeat(70 * 1024)}) : JSON.stringify({value: stage === 1 ? 1 : 0}),
    health: stage === 2 || stage === 4 ? 'unknown' : 'ok', firing: stage === 1};
}
export function quiet(config, now) {
  return config.mode === 'measure' && now >= config.epoch + 12 * HOUR && now < config.epoch + 18 * HOUR;
}
export function validateConfig(c, now) {
  if (!c || !/^[A-Za-z0-9-]{16,80}$/.test(c.runId) || !['warmup','measure','stopped'].includes(c.mode)) throw Error('invalid_config');
  if (!Number.isSafeInteger(c.epoch) || !Number.isSafeInteger(c.endsAt) || c.epoch > now + MINUTE || c.epoch < now - MINUTE) throw Error('invalid_epoch');
  const duration = c.endsAt - c.epoch;
  if (duration !== (c.mode === 'measure' ? 72 * HOUR : c.mode === 'warmup' ? 3 * HOUR : 0)) throw Error('invalid_duration');
  if (!/^[a-f0-9]{40}$/.test(c.applicationRevision) || !/^[a-f0-9]{40}$/.test(c.observerRevision)) throw Error('missing_revisions');
  const ids=c.watchIds||{status:'soak-status',projection:'soak-projection'};
  for(const kind of ['status','projection'])if(!new RegExp('^soak-'+kind+'(?:-[a-f0-9]{8})?$').test(ids[kind]))throw Error('invalid_watch_id');
  return {watchIds:ids,runId:c.runId,mode:c.mode,epoch:c.epoch,endsAt:c.endsAt,applicationRevision:c.applicationRevision,observerRevision:c.observerRevision};
}
export async function boundedJSON(response, max = 512 * 1024) {
  if (!response.body) throw Error('empty_body');
  const reader = response.body.getReader(), chunks = [];
  let size = 0;
  try {
    while (true) {
      const {done,value} = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > max) throw Error('response_too_large');
      chunks.push(value);
    }
  } catch (e) { await reader.cancel(); throw e; }
  const body = new Uint8Array(size); let offset = 0;
  for (const c of chunks) {body.set(c,offset);offset+=c.length;}
  return JSON.parse(new TextDecoder().decode(body));
}
export function expectedEvents(requests) {
  let open = false, unhealthy = false;
  const out=[];
  for (const r of [...requests].sort((a,b)=>a.at-b.at)) {
    const expected=fixture(r.kind,r.phase);
    const types=[];
    if (expected.health === 'unknown') {
      if (!unhealthy) types.push('source_error');
      unhealthy=true;
    } else {
      if (unhealthy) types.push('source_recovered');
      unhealthy=false;
      if (expected.firing && !open) types.push('firing');
      if (!expected.firing && open) types.push('recovered');
      open=expected.firing;
    }
    for(const type of types) out.push({type,requestId:r.id,at:r.at,kind:r.kind,phase:r.phase});
  }
  return out;
}
