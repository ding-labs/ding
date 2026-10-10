import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ErrorBox, useRead } from "../components/common";

type Model = { clientID: string; grant: { name: string; scopes: string[]; secretRefs: string[]; expiresAt: string; revoked: boolean } };
export function CloudModels() {
  const models = useRead<Model[]>("/cloud/models");
  const credentials = useRead<{ names: string[] }>("/cloud/secrets");
  const query = useQueryClient();
  const [name, setName] = useState(""); const [clientID, setClientID] = useState("");
  const [manage, setManage] = useState(false); const [refs, setRefs] = useState<string[]>([]);
  const [busy, setBusy] = useState(false); const [error, setError] = useState<unknown>();
  async function connect(e: FormEvent) {
    e.preventDefault(); setBusy(true); setError(undefined);
    try {
      await api("/cloud/models", { method: "POST", body: { clientID, grant: { name, days: 7, scopes: manage ? ["inspect", "preview", "manage", "retry"] : ["inspect", "preview"], secretRefs: refs } } });
      setName(""); setClientID(""); setRefs([]); await query.invalidateQueries();
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function disconnect(clientID: string) {
    setBusy(true); setError(undefined);
    try { await api("/cloud/models", { method: "POST", body: { clientID, revoke: true } }); await query.invalidateQueries(); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  return <section className="panel">
    <h2>Model connections</h2>
    <p>Connect your model client to <code>{window.location.origin}/mcp</code>. This source preview requires its registered OAuth client ID. Public marketplace installation is still being qualified.</p>
    <p>OAuth consent and this workspace grant must both permit an action. These seven-day grants can inspect watch data and preview changes. Enable management only when you want reviewed changes, lifecycle actions, and delivery retries.</p>
    <ErrorBox error={error || models.error} />
    <ul>{models.data?.map(m => <li key={m.clientID}><strong>{m.grant.name}</strong> — {m.grant.scopes.join(", ")}; expires {new Date(m.grant.expiresAt).toLocaleString()} <button disabled={busy} className="button" onClick={() => disconnect(m.clientID)}>Disconnect {m.grant.name}</button></li>)}</ul>
    <form onSubmit={connect} className="onboarding-form"><fieldset disabled={busy}>
      <label>Connection name<input required maxLength={100} value={name} onChange={e => setName(e.target.value)} /></label>
      <label>Registered OAuth client ID<input required maxLength={512} value={clientID} onChange={e => setClientID(e.target.value)} /></label>
      <label><input type="checkbox" checked={manage} onChange={e => setManage(e.target.checked)} />Allow reviewed changes and watch management</label>
      <p>Allow these credential references in model-created definitions. The model never receives their values.</p>
      {credentials.data?.names.map(ref => <label key={ref}><input type="checkbox" checked={refs.includes(ref)} onChange={e => setRefs(e.target.checked ? [...refs, ref] : refs.filter(r => r !== ref))} />{ref}</label>)}
      <button className="button primary" type="submit">Approve model connection</button>
    </fieldset></form>
  </section>;
}
