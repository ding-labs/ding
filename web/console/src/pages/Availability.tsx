import { useState } from "react";
import { api } from "../api/client";
import { useExecution } from "../app/execution";
import { preference, savePreference } from "../app/preferences";
import { ErrorBox, useRead } from "../components/common";

type Handoff = { id: string; watchId: string; role: string; phase: string; held: boolean };
type Preflight = { ready: boolean; reason?: string; manifest: string };
export function Availability({ watchID }: { watchID: string }) {
  const { mode } = useExecution();
  const [choice, setChoice] = useState(() => preference("ding.execution.preference", "local"));
  const [preflight, setPreflight] = useState<Preflight>();
  const [error, setError] = useState<unknown>(); const [busy, setBusy] = useState(false);
  const handoffs = useRead<Handoff[]>("/handoffs", 15000);
  const held = handoffs.data?.find(h => h.watchId === watchID && h.held);
  const arg = `'${watchID.replaceAll("'", "'\\''")}'`;
  function choose(value: string) { setChoice(value); savePreference("ding.execution.preference", value); }
  async function check() {
    setBusy(true); setError(undefined);
    try { setPreflight(await api(`/handoffs/preflight/${encodeURIComponent(watchID)}`)); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  return <section className="panel">
    <h2>Where this watch runs</h2>
    <p>{mode === "cloud" ? "Runs in Ding Cloud, independently of your computer." : "Runs on this computer while it is awake and Ding is running."}</p>
    {held ? <div className="notice" role="status"><strong>Execution is held by a transfer.</strong><p>Transfer <code>{held.id}</code> is {held.phase} on this {held.role}. Inspect both sides before resuming. An unreachable target does not prove it stopped.</p><code>ding cloud move status {held.id}</code></div> : null}
    <ErrorBox error={error || handoffs.error} />
    {mode === "local" ? <>
      <label>Execution preference<select value={choice} onChange={e => choose(e.target.value)}>
        <option value="local">Keep local — free, no account</option>
        <option value="server">Run on my own server — free, no account</option>
        <option value="cloud">Keep this watch running when my computer is off</option>
      </select></label>
      {choice === "server" ? <p>Use an always-on Mac mini or Linux host. Install Ding there, rebind credentials, and configure background startup. Test the destination before pausing this copy. The self-hosting guide covers boot startup, sleep settings, backups, and safe handoff. This choice stays saved; no account is needed.</p> : null}
      {choice === "cloud" ? <>
        <p>First check eligibility on this computer. No account is created and no configuration is uploaded. The current cloud preview accepts public HTTP watches at five-minute intervals or slower, with remote notification delivery.</p>
        <button className="button" disabled={busy || Boolean(held)} onClick={() => void check()}>{busy ? "Checking locally…" : "Check eligibility locally"}</button>
        {preflight ? <div className="notice"><strong>{preflight.ready ? "Eligible for the cloud preview" : "Keep running locally"}</strong><p>{preflight.reason || "Moving uploads this watch’s definition and explicitly rebound credentials. It starts fresh evaluation state and may briefly interrupt monitoring. Local history remains here."}</p>
          {preflight.ready ? <><details><summary>Review the exact configuration</summary><pre>{preflight.manifest}</pre></details><p>Connect your chosen cloud service with <code>ding cloud login --url HTTPS_ORIGIN</code>. Rebind credentials in its Console, then prepare the move:</p><pre>ding cloud move prepare {arg} --yes</pre><p>The command prints a transfer ID and waits for a separate delivery-confirmation step before pausing local execution. Public cloud enrollment and marketplace availability are still being qualified.</p></> : null}
        </div> : null}
      </> : null}
    </> : <><p>To return to the original local installation, rebind its credentials and prepare a return there:</p><pre>ding cloud move prepare {arg} --back --yes</pre><p>Cloud execution continues until you confirm the local delivery test and finish the handoff. You can also export the definition above for independent self-hosting.</p></>}
  </section>;
}
