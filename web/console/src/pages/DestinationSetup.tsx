import { useRef, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, logout } from "../api/client";
import type { WatchrunDestinationSetupStatus } from "../api/contracts";
import { useCapability, useExecution } from "../app/execution";
import { Heading, ErrorBox } from "../components/common";
import { SavedDestinationPicker } from "../components/SavedDestinationPicker";
import { DestinationCredential } from "../components/DestinationCredential";

export function DestinationSetup() {
  const [params, setParams] = useSearchParams();
  const execution = useExecution();
  const allowed = useCapability("destinations");
  const mismatch = !!((params.get("account") && params.get("account") !== execution.user?.id) || (params.get("workspace") && params.get("workspace") !== execution.workspace));
  const ref = params.get("destination") || "";
  const client = useQueryClient();
  const status = useQuery({ queryKey: ["destination-setup", execution.user?.id, execution.workspace, ref], enabled: !mismatch,
    queryFn: ({ signal }) => api<WatchrunDestinationSetupStatus>(`/onboarding/destination-status?destination=${encodeURIComponent(ref)}`, { signal }) });
  const [id, setID] = useState("");
  const [type, setType] = useState(execution.mode === "local" ? "console" : "webhook");
  const [credentialReady, setCredentialReady] = useState(false);
  const [credential, setCredential] = useState("DING_ALERTS");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  function select(value: string) { const next = new URLSearchParams(params); value ? next.set("destination", value) : next.delete("destination"); setParams(next); setError(undefined); }
  async function create(event: FormEvent) {
    event.preventDefault(); if (!status.data || mismatch || !allowed) return;
    setBusy(true); setError(undefined);
    try {
      await api("/onboarding/destination", { method: "POST", body: { accountID: status.data.accountID, workspaceID: status.data.workspaceID, id, type, credential: ["console", "desktop"].includes(type) ? "" : credential } });
      await client.invalidateQueries({ queryKey: ["onboarding-destinations"] }); select(id);
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  return <>
    <Heading eyebrow="Notification setup" title="Choose where Ding should alert you" description="Set up and confirm delivery, then return to your conversation to review the watch." />
    <p>Signed in as {execution.user?.name || "your account"}. Workspace: <code>{execution.workspace}</code>.</p>
    {mismatch ? <section className="panel" role="alert"><h2>This setup link belongs to another account or workspace</h2>
      <p>Use the workspace selector or sign in to the account connected in your conversation, then reopen the setup link. This page will not change the current workspace.</p>
      {execution.mode !== "local" && <button className="button" onClick={async () => { await logout(); window.location.reload(); }}>Sign out and choose the intended account</button>}
    </section> : <>
      <ErrorBox error={error || status.error} />
      {!allowed && <p>An administrator must set up and confirm delivery for this account.</p>}
      <section className="panel"><SavedDestinationPicker selectedRef={ref} onChange={d => select(d?.definition.metadata.id || "")} disabled={busy} />
      {!ref && <form className="onboarding-form" onSubmit={create}><fieldset disabled={busy || !allowed || !status.data}>
        <label>Destination ID<input required value={id} maxLength={64} onChange={e => setID(e.target.value)} /></label>
        <label>Delivery type<select value={type} onChange={e => setType(e.target.value)}>
          <option value="webhook">Webhook</option><option value="slack">Slack incoming webhook</option><option value="discord">Discord webhook</option>
          {execution.mode !== "cloud" && <><option value="console">Console output</option><option value="desktop">Desktop notification</option></>}
        </select></label>
        {!["console", "desktop"].includes(type) && <DestinationCredential name={credential} onName={setCredential} onReady={setCredentialReady} disabled={busy || !allowed} />}
        <button className="button primary" type="submit" disabled={!["console", "desktop"].includes(type) && !credentialReady}>Create notification destination</button>
      </fieldset></form>}
      </section>
      {ref && status.data && <DestinationReceipt key={`${status.data.accountID}:${status.data.workspaceID}:${ref}:${status.data.revision}`} status={status.data} refresh={async () => { await status.refetch(); }} allowed={allowed} />}
    </>}
    <p>No watch is created here. <Link to="/">Leave setup</Link> or return to your conversation whenever you are ready.</p>
  </>;
}

function DestinationReceipt({ status, refresh, allowed }: { status: WatchrunDestinationSetupStatus; refresh: () => Promise<void>; allowed: boolean }) {
  const { mode } = useExecution();
  const storageKey = `ding.setup-test:${status.accountID}:${status.workspaceID}:${status.destinationRef}:${status.revision}`;
  const operation = useRef("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  async function send(another = false) {
    setBusy(true); setError(undefined);
    try {
      // Keep an in-flight key across reloads. Readiness and authorization remain server-owned.
      operation.current ||= sessionStorage.getItem(storageKey) || status.probe?.id || "";
      if (another || !operation.current || (operation.current === status.probe?.id && ["expired", "stale", "failed"].includes(status.state))) operation.current = crypto.randomUUID();
      sessionStorage.setItem(storageKey, operation.current);
      await api(mode === "local" ? "/onboarding/destination-test" : "/cloud/destination-test", { method: "POST", body: { accountID: status.accountID, workspaceID: status.workspaceID, destinationRef: status.destinationRef, revision: status.revision, operationKey: operation.current } });
      await refresh();
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function confirm() {
    setBusy(true); setError(undefined);
    try { await api("/onboarding/destination-confirm", { method: "POST", body: { accountID: status.accountID, workspaceID: status.workspaceID, destinationRef: status.destinationRef, revision: status.revision, operationKey: status.probe?.id } }); await refresh(); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  const messages: Record<string, string> = {
    missing: "This saved destination is missing. Select another destination or create one.", unavailable: "The destination or its credential is unavailable. Check its configuration and refresh.",
    untested: "Send a labeled test to check this destination.", pending: "The test outcome is uncertain. Check the destination and refresh its status. Ding will not resend this test automatically.",
    failed: "The destination rejected the test. Check its configuration before sending another labeled test.", accepted: "The destination accepted the test. Confirm only if you received it.",
    confirmed: "Receipt confirmed. Return to your conversation and ask Ding to refresh notification readiness and preview your service watch.",
    stale: "The destination or its credentials changed. Send a new labeled test before confirming receipt.", expired: "This confirmation window expired. Send a new labeled test before confirming receipt.",
  };
  return <section className="panel"><h2>Confirm notification delivery</h2><p role="status" aria-label="Notification readiness">{messages[status.state]}</p><ErrorBox error={error} />
    <button className="button" type="button" disabled={busy} onClick={refresh}>Refresh delivery status</button>
    {!["confirmed", "accepted", "missing", "unavailable", "pending"].includes(status.state) && <button className="button" disabled={busy || !allowed} onClick={() => void send()}>{operation.current ? "Check this test outcome" : ["stale", "expired", "failed"].includes(status.state) ? "Send a new labeled test" : "Send labeled notification test"}</button>}
    {status.state === "pending" && <button className="button" disabled={busy || !allowed} onClick={() => { if (window.confirm("The earlier test may have arrived. Send another labeled test?")) void send(true); }}>Send another test (may duplicate)</button>}
    {status.state === "unavailable" && <Link to={mode === "local" ? "/workbench" : "/system?tab=Cloud"}>Check destination credentials</Link>}
    {status.state === "accepted" && <button className="button primary" disabled={busy || !allowed} onClick={confirm}>I received the test at this destination</button>}
    {status.expiresAt && <p>Valid until {new Date(status.expiresAt).toLocaleString()}. Changes to the destination or credentials require another test.</p>}
    <p className="muted">Acceptance alone does not prove receipt. Console tests appear in daemon output. Return to your conversation manually; this page does not reopen it.</p>
  </section>;
}
