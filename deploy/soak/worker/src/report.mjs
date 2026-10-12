import {expectedEvents,MINUTE,HOUR,watchIds} from './core.mjs';
const stamp=x=>typeof x==='number'?x:Date.parse(x);
const gate=(verdict,details)=>({verdict,details});
export function report(input,now=Date.now()) {
  const {config}=input;
  if(!config)return {ready:false,gates:{evidence:gate('inconclusive','No run manifest')}};
  const end=Math.min(now,config.endsAt),start=config.epoch;
  const samples=(input.samples||[]).filter(s=>s.scheduledAt>=start&&s.scheduledAt<end).sort((a,b)=>a.scheduledAt-b.scheduledAt);
  const byMinute=new Map(samples.map(s=>[s.scheduledAt,s]));
  const expected=[];for(let m=Math.ceil(start/MINUTE)*MINUTE;m<end-MINUTE;m+=MINUTE)expected.push(m);
  const missing=expected.filter(m=>!byMinute.has(m));
  let gap=0,maxGap=0;for(const m of expected){gap=byMinute.has(m)?0:gap+1;maxGap=Math.max(maxGap,gap);}
  const maintenance=input.maintenance||[];
  const planned=t=>maintenance.some(w=>stamp(w.declaredAt)<stamp(w.start)&&t>=stamp(w.start)&&t<stamp(w.end));
  const failures=samples.filter(s=>!s.health?.ok||s.health.data.status!==200);
  const unexpectedHealth=failures.filter(s=>!planned(s.startedAt));
  const errors=[];for(const s of samples) {
    for(const [key,v] of Object.entries({...s.ding,railway:s.railway,usage:s.usage,doctor:s.doctor})) if(v&&(!v.ok||v.data?.more===true))errors.push({at:s.startedAt,collector:key,error:v.error||'backlog',planned:planned(s.startedAt)});
  }
  const requests=(input.fixtures||[]).filter(r=>r.at>=start&&r.at<end-15000);
  const events=(input.events||[]).filter(e=>stamp(e.event.at)>=start&&stamp(e.event.at)<end);
  const correctness=[];const phaseCoverage={};
  for(const kind of ['status','projection']) {
    const rr=requests.filter(r=>r.kind===kind).sort((a,b)=>a.at-b.at);
    phaseCoverage[kind]=Array.from({length:6},(_,p)=>rr.filter(r=>r.phase===p).length);
    const actual=events.filter(e=>e.event.watchId===(config.watchIds?.[kind]||'soak-'+kind)&&['firing','recovered','source_error','source_recovered'].includes(e.event.type));
    const used=new Set();
    for(const expectedEvent of expectedEvents(rr)) {
      const matches=actual.filter(e=>!used.has(e.event.id)&&e.event.type===expectedEvent.type&&stamp(e.event.at)>=expectedEvent.at-2000&&stamp(e.event.at)<=expectedEvent.at+15000);
      if(matches.length!==1){correctness.push({...expectedEvent,error:'missing_or_duplicate_event',matches:matches.length});continue;}
      const e=matches[0];used.add(e.event.id);
      if(!e.observations?.some(o=>stamp(o.acceptedAt)>=expectedEvent.at-2000&&stamp(o.acceptedAt)<=expectedEvent.at+15000))correctness.push({...expectedEvent,error:'missing_observation'});
    }
    for(const e of actual)if(!used.has(e.event.id))correctness.push({error:'unexpected_event',id:e.event.id,type:e.event.type,at:e.event.at});
  }
  const due=new Map(),seenInputs=new Set(),scheduling=[];
  let observable=0,onTime=0;
  for(const s of samples) {
    if(s.quiet){due.clear();continue;}
    for(const id of watchIds(config)) {
    const w=s.ding?.[id]?.data?.watch;if(!w)continue;
    const at=stamp(w.lastInputAt),key=id+'/'+w.lastInputAt;
    if(!seenInputs.has(key)) {
      seenInputs.add(key);const next=due.get(id);
      if(next&&at>=next-1000&&!maintenance.some(w=>stamp(w.start)<at&&stamp(w.end)>next)) {observable++;if(at-next<=MINUTE)onTime++;else scheduling.push({id,at,latenessSeconds:(at-next)/1000});}
    }
    const next=stamp(w.nextAt);
    if(Number.isFinite(next)) {
      due.set(id,next);
      if(s.startedAt-next>5*MINUTE&&!planned(s.startedAt))scheduling.push({id,at:s.startedAt,overdueSeconds:(s.startedAt-next)/1000});
    }
  }
  }
  // Dedupe overlapping Railway export windows by metric/timestamp.
  const series={};
  for(const s of samples)if(s.railway?.ok)for(const r of [...(s.railway.data.metrics||[]),...(s.railway.data.disk||[])]) {
    series[r.measurement]??=new Map();for(const v of r.values||[])series[r.measurement].set(v.ts,v.value);
  }
  const resources={};const excursions=[];
  for(const [name,values] of Object.entries(series)) {
    const rows=[...values].filter(([ts])=>ts*1000>=start&&ts*1000<end).sort((a,b)=>a[0]-b[0]);
    const nums=rows.map(r=>r[1]);
    resources[name]={samples:nums.length,min:Math.min(...nums),max:Math.max(...nums),mean:nums.reduce((a,b)=>a+b,0)/(nums.length||1)};
    const threshold={CPU_USAGE:0.5,MEMORY_USAGE_GB:0.5,DISK_USAGE_GB:0.7}[name];
    let first=null,previous=null;
    for(const [ts,value] of rows){if(value>threshold){if(first===null||ts-previous>120)first=ts;if(ts-first>=15*60)excursions.push({name,ts,value});previous=ts;}else{first=null;previous=null;}}
  }
  const deliveries=samples.some(s=>s.doctor?.data?.usage?.pending>0||Object.values(s.ding||{}).some(v=>v.data?.deliveries?.some(d=>['pending','leased'].includes(d.status))));
  const watchHealthy=watchIds(config).every(id=>{const d=samples.at(-1)?.ding?.[id]?.data;return d?.watch.status==='running'&&(id==='freedesign-http'||!d.entities?.some(e=>e.open||e.sourceUnhealthy));});
  const covered=Object.values(phaseCoverage).every(counts=>counts.every(n=>n>=2));
  const metricsComplete=['CPU_USAGE','MEMORY_USAGE_GB','DISK_USAGE_GB','NETWORK_RX_GB','NETWORK_TX_GB'].every(n=>resources[n]?.samples>0);
  const fraction=expected.length?(expected.length-missing.length)/expected.length:0;
  const elapsedHours=(end-start)/HOUR;
  const drills=input.drills||{};
  const gates={
    evidence:gate(!expected.length?'inconclusive':fraction>=0.99&&maxGap<=5?(errors.some(e=>!e.planned)?'inconclusive':'pass'):'fail',{expected:expected.length,collected:expected.length-missing.length,fraction,maxGapMinutes:maxGap,collectorErrors:errors}),
    availability:gate(unexpectedHealth.length?'fail':!samples.length?'inconclusive':'pass',{failedSamples:failures.length,unplanned:unexpectedHealth.length,rawSampleDowntimeMinutes:failures.length,note:'Minute probes bound downtime; they do not measure exact outage duration.'}),
    scheduling:gate(!observable?'inconclusive':onTime/observable>=0.99&&!scheduling.some(x=>x.overdueSeconds>300)?'pass':'fail',{observable,onTime,fraction:observable?onTime/observable:null,exceptions:scheduling,quietWindow:'Due-time sampling intentionally excluded; reconcile durable history at recovery checkpoint.'}),
    correctness:gate(correctness.length?'fail':covered?'pass':'inconclusive',{phaseCoverage,mismatches:correctness}),
    consoleOnly:gate(deliveries?'fail':samples.some(s=>s.doctor?.ok)?'pass':'inconclusive','Pending deliveries must remain zero; manifest destinations are checked during preparation.'),
    resources:gate(excursions.length?'inconclusive':metricsComplete?'pass':'inconclusive',{series:resources,excursions,note:'Container memory, not process RSS. Growth, quota and OOM review also required at operator checkpoints.'}),
    persistence:gate(drills.persistence?.verdict||'inconclusive',drills.persistence||'Restart checkpoint not recorded'),
    accessRecovery:gate(drills.accessRecovery?.verdict||'inconclusive',drills.accessRecovery||'Access and isolated recovery checkpoints not recorded')
  };
  const elapsed=now>=config.endsAt;
  const issues=[...unexpectedHealth.map(s=>({kind:'health',at:s.startedAt})),...errors,...correctness,...scheduling,...excursions];
  if(input.safety){gates.safety=gate('fail',input.safety);issues.push(input.safety);}
  const warmupReady=!input.safety&&elapsedHours>=1.5&&covered&&correctness.length===0&&fraction>=0.99&&maxGap<=5&&errors.length===0&&unexpectedHealth.length===0&&metricsComplete&&watchHealthy;
  return {runId:config.runId,mode:config.mode,from:new Date(start).toISOString(),through:new Date(end).toISOString(),endsAt:new Date(config.endsAt).toISOString(),elapsedHours,elapsed,ready:config.mode==='warmup'&&warmupReady,gates,issues,
    verdict:config.mode==='measure'&&elapsed&&Object.values(gates).every(g=>g.verdict==='pass')&&drills.resourceReview?.verdict==='pass'&&drills.billing?.verdict==='pass'?'pass':Object.values(gates).some(g=>g.verdict==='fail')?'fail':'inconclusive'};
}
