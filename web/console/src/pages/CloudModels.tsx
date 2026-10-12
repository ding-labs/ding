import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useExecution, useCapability } from "../app/execution";
import { api, download } from "../api/client";
import { ErrorBox, useRead } from "../components/common";

type Model = { clientID: string; grant: { name: string; scopes: string[]; secretRefs: string[]; expiresAt: string; revoked: boolean } };
export function CloudModels() {
  const mayMove = useCapability("move");
  const transferEnabled = useExecution().accountFeatures?.modelTransfers;
  const [move, setMove] = useState(false);
  const mayPreview = useCapability("preview");
  const mayManage = useCapability("manage");
  const mayUseCredentials = useCapability("credentials");
  const shared = useExecution().mode === "shared";
  const [pairing, setPairing] = useState<unknown>();
  const models = useRead<Model[]>("/cloud/models");
  const credentials = useRead<{ names: string[] }>("/cloud/secrets",false,mayUseCredentials);
  const query = useQueryClient();
  const [name, setName] = useState(""); const [clientID, setClientID] = useState("");
  const [manage, setManage] = useState(false); const [refs, setRefs] = useState<string[]>([]);
  const [busy, setBusy] = useState(false); const [error, setError] = useState<unknown>();
  async function connect(e: FormEvent) {
    e.preventDefault(); setBusy(true); setError(undefined);
    try {
      const result = await api<{connection?: unknown}>("/cloud/models", { method: "POST", body: { local: shared, clientID, grant: { name, days: 7, scopes: manage ? ["inspect", "preview", "manage", "retry", ...(move ? ["move"] : [])] : mayPreview ? ["inspect", "preview"] : ["inspect"], secretRefs: refs } } });
      setPairing(result.connection);
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
    {!shared && <p>Connect your model client to <code>{window.location.origin}/mcp</code>. This source preview requires its registered OAuth client ID. Public marketplace installation is still being qualified.</p>}
    {shared && <p>Create a private pairing file, then run <code>ding mcp serve --config /path/to/connection.json</code> in your agent’s local MCP configuration. Keep the file private with permissions 0600.</p>}
    {pairing != null && <div role="status"><p>This file contains the agent credential and is available only now.</p><button className="button" onClick={() => {download("ding-agent.json",JSON.stringify(pairing,null,2));setPairing(undefined);}}>Download private pairing file</button><button className="button" onClick={() => setPairing(undefined)}>Dismiss</button></div>}
    <p>{shared ? "Current membership" : "OAuth consent"} and this workspace grant must both permit an action. These seven-day grants can inspect watch data and preview changes. Enable management only when you want reviewed changes, lifecycle actions, and delivery retries.</p>
    <ErrorBox error={error || models.error} />
    <ul>{models.data?.map(m => <li key={m.clientID}><strong>{m.grant.name}</strong> — {m.grant.scopes.join(", ")}; expires {new Date(m.grant.expiresAt).toLocaleString()} <button disabled={busy} className="button" onClick={() => disconnect(m.clientID)}>Disconnect {m.grant.name}</button></li>)}</ul>
    <form onSubmit={connect} className="onboarding-form"><fieldset disabled={busy}>
      <label>Connection name<input required maxLength={100} value={name} onChange={e => setName(e.target.value)} /></label>
      {!shared && <label>Registered OAuth client ID<input required maxLength={512} value={clientID} onChange={e => setClientID(e.target.value)} /></label>}
      {mayManage && <label><input type="checkbox" checked={manage} onChange={e => {setManage(e.target.checked);if(!e.target.checked)setMove(false);}} />Allow reviewed changes and watch management</label>}
      {!shared && transferEnabled && mayMove && <label><input type="checkbox" checked={move} disabled={!manage} onChange={e => setMove(e.target.checked)} />Allow reviewed watch transfers through the approved Ding bridge client</label>}
      <p>Allow these credential references in model-created definitions. The model never receives their values.</p>
      {credentials.data?.names.map(ref => <label key={ref}><input type="checkbox" checked={refs.includes(ref)} onChange={e => setRefs(e.target.checked ? [...refs, ref] : refs.filter(r => r !== ref))} />{ref}</label>)}
      <button className="button primary" type="submit">Approve model connection</button>
    </fieldset></form>
  </section>;
}
