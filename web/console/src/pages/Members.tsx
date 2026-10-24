import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { useExecution, useCapability } from "../app/execution";
import { ErrorBox, useRead } from "../components/common";
type Member = { id: string; name: string; username?: string; role: string; disabled: boolean };
const memberName = (member: Member) => member.name || member.username || "Name not set";
export function Members() {
  const passwords = useExecution().accountFeatures?.passwords; const owner = useCapability("owners");
  const members = useRead<Member[]>("/members"); const query = useQueryClient();
  const [username, setUsername] = useState(""); const [name, setName] = useState(""); const [role, setRole] = useState("viewer");
  const [link, setLink] = useState(""); const [error, setError] = useState<unknown>(); const [busy, setBusy] = useState(false);
  const roles = owner ? ["viewer", "editor", "administrator", "owner"] : ["viewer", "editor"];
  async function change(path: string, method: string, body?: unknown) {
    setBusy(true); setError(undefined); setLink("");
    try { const out = await api<{url?: string}>(path, {method, body}); setLink(out.url || ""); await query.invalidateQueries({queryKey:["/members"]}); await members.refetch(); }
    catch (e) { setError(e); } finally {setBusy(false);}
  }
  function invite(e: FormEvent) {e.preventDefault(); void change("/members", "POST", passwords ? {username,name,role} : {user:username,role});}
  return <section className="panel"><h2>Workspace members</h2><p>Viewers inspect. Editors manage watches. Administrators manage credentials and members. Owners also manage ownership and workspace deletion.</p>
    <ErrorBox error={error || members.error}/>
    {link && <div role="status"><p>Share this one-use link privately. It expires in one hour.</p><input aria-label="Account activation link" readOnly value={link}/><button className="button" onClick={() => setLink("")}>Dismiss</button></div>}
    <ul>{members.data?.map(m => <li key={m.id}><strong>{memberName(m)}</strong> {m.username || m.id} {m.disabled ? "(disabled)" : ""}{" "}
      <select aria-label={`Role for ${memberName(m)}`} value={m.role} disabled={busy || (!owner && !roles.includes(m.role))} onChange={e => void change(`/members/${m.id}`, "PATCH", {role:e.target.value})}>{[...new Set([...roles,m.role])].map(r => <option key={r}>{r}</option>)}</select>{" "}
      {(owner || roles.includes(m.role)) && <>
        <button className="button" disabled={busy} onClick={() => void change(`/members/${m.id}`,"PATCH",{disabled:!m.disabled})}>{m.disabled ? "Enable" : "Disable"}</button>{" "}
        {passwords && m.role !== "owner" && <button className="button" disabled={busy} onClick={() => void change(`/members/${m.id}`,"PATCH",{reset:true})}>Reset password</button>}{" "}
        <button className="button" disabled={busy} onClick={() => {if(confirm(`Remove ${memberName(m)} from this workspace?`)) void change(`/members/${m.id}`,"DELETE");}}>Remove</button>
      </>}
    </li>)}</ul>
    <form className="onboarding-form" onSubmit={invite}><fieldset disabled={busy}>
      <label>{passwords ? "New username" : "Existing user ID"}<input required value={username} onChange={e=>setUsername(e.target.value)}/></label>
      {passwords && <label>Display name<input required maxLength={100} value={name} onChange={e=>setName(e.target.value)}/></label>}
      <label>Role<select value={role} onChange={e=>setRole(e.target.value)}>{roles.map(r=><option key={r}>{r}</option>)}</select></label>
      <button className="button primary">{passwords ? "Create invitation" : "Add member"}</button>
    </fieldset></form>
  </section>;
}

export function UserSecurity() {
  const {accountFeatures,user} = useExecution();
  const sessions = useRead<{id:string;kind:string;workspace:string;expiresAt:string;current:boolean}[]>("/user/sessions");
  const [current,setCurrent]=useState("");const [password,setPassword]=useState("");const [error,setError]=useState<unknown>();const [busy,setBusy]=useState(false);
  async function revoke(id:string, active:boolean) {setBusy(true);try{await api(`/user/sessions/${id}`,{method:"DELETE"});if(active) window.location.reload();else await sessions.refetch();}catch(e){setError(e);}finally{setBusy(false);}}
  async function change(e:FormEvent) {e.preventDefault();setBusy(true);try{await api("/user/password",{method:"POST",body:{current,password}});window.location.reload();}catch(e){setError(e);}finally{setBusy(false);}}
  return <section className="panel"><h2>Your account</h2><p>{user?.name} · <code>{user?.id}</code></p><ErrorBox error={error || sessions.error}/>
    {accountFeatures?.passwords && user?.username && <form className="onboarding-form" onSubmit={change}><fieldset disabled={busy}>
      <label>Current password<input required type="password" autoComplete="current-password" value={current} onChange={e=>setCurrent(e.target.value)}/></label>
      <label>New password<input required minLength={12} maxLength={1024} type="password" autoComplete="new-password" value={password} onChange={e=>setPassword(e.target.value)}/></label>
      <button className="button">Change password and sign out everywhere</button>
    </fieldset></form>}
    <h3>Active sessions</h3><ul>{sessions.data?.map(s=><li key={s.id}>{s.kind}{s.current?" (this browser)":""} · expires {new Date(s.expiresAt).toLocaleString()} <button className="button" disabled={busy} onClick={()=>void revoke(s.id,s.current)}>Revoke</button></li>)}</ul>
  </section>;
}
