import { useState } from "react";
import { api } from "../api/client";
import { useExecution } from "../app/execution";
import { ErrorBox } from "../components/common";

export function CloudAccount() {
  const { workspace } = useExecution();
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false); const [error, setError] = useState<unknown>();
  async function exportAll() {
    setBusy(true); setError(undefined);
    try {
      const data = await api("/cloud/export");
      const object = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: "application/json" }));
      const link = document.createElement("a"); link.href = object; link.download = "ding-workspace-definitions.json"; link.click(); URL.revokeObjectURL(object);
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function remove() {
    setBusy(true); setError(undefined);
    try { await api("/cloud/account", { method: "DELETE", body: { confirmWorkspace: workspace } }); window.location.assign("/ui/?account=deleted"); }
    catch (e) { setError(e); setBusy(false); }
  }
  return <section className="panel">
    <h2>Export or delete this workspace</h2>
    <p>Download watch definitions without secret values. Rebind credentials when importing them elsewhere. Exporting does not transfer execution ownership.</p>
    <button className="button" disabled={busy} onClick={() => void exportAll()}>Export all definitions</button>
    <h3>Delete account and hosted watches</h3>
    <p>This permanently stops hosted execution, revokes sessions and model access, and removes live workspace data and credentials. Encrypted backups follow the service operator’s disclosed retention. Local copies are independent.</p>
    <p>If you moved a local watch here, move it back before deleting this account. Deletion does not release a local transfer hold or automatically resume a local copy.</p>
    <ErrorBox error={error} />
    <label>Type DELETE to confirm<input value={confirmation} onChange={e => setConfirmation(e.target.value)} autoComplete="off" disabled={busy} /></label>
    <button className="button danger" disabled={busy || confirmation !== "DELETE" || !workspace} onClick={() => void remove()}>Delete workspace and stop hosted watches</button>
  </section>;
}
