import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CloudAccount } from "./CloudAccount";
import { CloudModels } from "./CloudModels";
import { api } from "../api/client";
import { useRead, ErrorBox, Loading } from "../components/common";

type Usage = { usage: { period: string; checks: number; deliveries: number; bytes: number }; limits: { checks: number; deliveryAttempts: number; bytes: number }; note: string };

export function CloudSystem() {
  const usage = useRead<Usage>("/cloud/usage", 30000);
  const names = useRead<{ names: string[] }>("/cloud/secrets");
  const [name, setName] = useState(""); const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false); const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState(""); const client = useQueryClient();
  async function save(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(undefined); setMessage("");
    try { await api("/cloud/secrets", { method: "POST", body: { name, value } }); setValue(""); setMessage(`Saved ${name}. Watches resolve the new value on their next attempt.`); await client.invalidateQueries(); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function remove(name: string) {
    if (!confirm(`Delete ${name}? Watches using this credential will report it missing until you replace it.`)) return;
    setBusy(true); setError(undefined); setMessage("");
    try { await api(`/cloud/secrets/${encodeURIComponent(name)}`, { method: "DELETE" }); await client.invalidateQueries(); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  const mib = (n: number) => (n / (1 << 20)).toFixed(1);
  return <>
    <section className="panel">
      <h2>Hosted usage</h2>
      <p>These watches run independently of your computer. This preview has no uptime SLA. Local and self-hosted Ding remain complete alternatives.</p>
      <ErrorBox error={usage.error} />
      {usage.isPending ? <Loading /> : usage.data ? <>
        <p>UTC calendar month {usage.data.usage.period}</p>
        <dl className="facts">
          <div><dt>Check attempts</dt><dd>{usage.data.usage.checks} / {usage.data.limits.checks}</dd></div>
          <div><dt>Delivery attempts</dt><dd>{usage.data.usage.deliveries} / {usage.data.limits.deliveryAttempts}</dd></div>
          <div><dt>Metered traffic</dt><dd>{mib(usage.data.usage.bytes)} / {mib(usage.data.limits.bytes)} MiB</dd></div>
        </dl>
        <p className="muted">{usage.data.note}</p>
        <p>When an outbound budget is exhausted, checks and deliveries cannot contact their destinations. Ding reports that failure instead of showing successful monitoring. Inspect Diagnostics for source, outbox and storage pressure.</p>
      </> : null}
    </section>
    <section className="panel">
      <h2>Workspace credentials</h2>
      <p>Values are encrypted in this workspace and are never returned to the Console or a model. A watch export contains references, so moving back to your own machine requires rebinding credentials.</p>
      <ErrorBox error={error || names.error} />
      {message ? <p role="status">{message}</p> : null}
      <ul>{names.data?.names.map(name => <li key={name}><code>{name}</code>{" "}<button className="button" disabled={busy} onClick={() => remove(name)}>Delete {name}</button></li>)}</ul>
      <form className="onboarding-form" onSubmit={save}><fieldset disabled={busy}>
        <label>Credential name<input required value={name} pattern="[A-Za-z_][A-Za-z0-9_]*" maxLength={128} onChange={e => setName(e.target.value)} /></label>
        <label>New value<input required type="password" autoComplete="new-password" value={value} onChange={e => setValue(e.target.value)} /></label>
        <button className="button primary" type="submit">Save or replace credential</button>
      </fieldset></form>
    </section>
    <CloudModels />
    <CloudAccount />
  </>;
}
