import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import { useExecution } from "../app/execution";
import { ErrorBox, useRead } from "../components/common";
type Service = {id:string;name:string;scopes:string[];watches:string[];expiresAt:string;token?:string};
export function Services() {
 const shared=useExecution().mode==="shared";
 const list=useRead<Service[]>("/services"); const[name,setName]=useState("");const[watch,setWatch]=useState("");const[scope,setScope]=useState("inspect");const[token,setToken]=useState("");const[busy,setBusy]=useState(false);const[error,setError]=useState<unknown>();
 async function create(e:FormEvent){e.preventDefault();setBusy(true);setToken("");try{const result=await api<Service>("/services",{method:"POST",body:{name,scopes:[scope],watches:scope==="ingest"?watch.split(",").map(v=>v.trim()).filter(Boolean):[],days:30}});setToken(result.token||"");await list.refetch();}catch(e){setError(e);}finally{setBusy(false);}}
 async function revoke(id:string){setBusy(true);setToken("");try{await api(`/services/${id}`,{method:"DELETE"});await list.refetch();}catch(e){setError(e);}finally{setBusy(false);}}
 return <section className="panel"><h2>Service credentials</h2><p>Workspace-owned automation keeps running when its creator leaves. These credentials expire in 30 days. Revoke them here when no longer needed.</p><ErrorBox error={error||list.error}/>
 {token&&<div role="status"><p>Save this credential privately now. It is shown only once.</p><input aria-label="New service credential" readOnly value={token}/><button className="button" onClick={()=>setToken("")}>Dismiss</button></div>}
 <ul>{list.data?.map(s=><li key={s.id}>{s.name} · {s.scopes.join(", ")} · expires {new Date(s.expiresAt).toLocaleString()} <button className="button" disabled={busy} onClick={()=>void revoke(s.id)}>Revoke</button></li>)}</ul>
 <form className="onboarding-form" onSubmit={create}><fieldset disabled={busy}><label>Name<input required maxLength={100} value={name} onChange={e=>setName(e.target.value)}/></label><label>Permission<select value={scope} onChange={e=>setScope(e.target.value)}><option value="inspect">Inspect this workspace</option><option value="retry">Retry deliveries in this workspace</option>{shared&&<option value="ingest">Send input to selected watches</option>}</select></label>{scope==="ingest"&&<label>Ingest watch IDs, separated by commas<input required value={watch} onChange={e=>setWatch(e.target.value)}/></label>}<button className="button">Create credential</button></fieldset></form>
 </section>;
}
type AuditRow={sequence:number;actor:string;action:string;target:string;at:string};
export function Audit(){
 const[source,setSource]=useState("runtime");const[before,setBefore]=useState(0);
 const records=useRead<AuditRow[]>(`/audit?source=${source}&before=${before}`);
 return <section className="panel"><h2>Workspace audit</h2><p>Committed changes are recorded with their actor. Passwords, tokens, and credential values are excluded. Each store retains its latest 10,000 records.</p><label>History<select value={source} onChange={e=>{setSource(e.target.value);setBefore(0);}}><option value="runtime">Watches and operations</option><option value="identity">Members and credentials</option></select></label><ErrorBox error={records.error}/><ul>{records.data?.map(a=><li key={a.sequence}>{new Date(a.at).toLocaleString()} · {a.actor} · {a.action} <code>{a.target}</code></li>)}</ul><button className="button" disabled={!before} onClick={()=>setBefore(0)}>Latest</button>{" "}<button className="button" disabled={records.data?.length!==100} onClick={()=>setBefore(records.data?.at(-1)?.sequence||0)}>Older</button></section>;
}
