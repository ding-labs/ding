import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { useRead, ErrorBox } from "./common";
import { useCapability, useExecution } from "../app/execution";

export function DestinationCredential({ name, onName, disabled, onReady }: { name: string; onName: (value: string) => void; disabled: boolean; onReady: (ready: boolean) => void }) {
  const { mode } = useExecution();
  const allowed = useCapability("credentials");
  const names = useRead<{ names: string[] }>("/cloud/secrets", false, mode !== "local" && allowed);
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const client = useQueryClient();
  useEffect(() => { onReady(mode === "local" ? !!name : !!names.data?.names.includes(name)); }, [mode, name, names.data, onReady]);
  async function save() {
    setBusy(true); setError(undefined);
    try { await api("/cloud/secrets", { method: "POST", body: { name, value } }); setValue(""); await client.invalidateQueries({ queryKey: ["/cloud/secrets"] }); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  return <fieldset disabled={disabled || busy}>
    <label>Credential reference<input required value={name} pattern="[A-Za-z_][A-Za-z0-9_]*" maxLength={128} onChange={e => { onName(e.target.value); setValue(""); }} /></label>
    <ErrorBox error={error || names.error} />
    {mode === "local" ? <p>Use an environment credential configured on the computer running Ding.</p> : names.data?.names.includes(name) ? <p>This workspace credential is saved. Its value stays out of chat.</p> : allowed ? <>
      <label>Destination webhook URL<input type="password" autoComplete="new-password" value={value} onChange={e => setValue(e.target.value)} /></label>
      <p>Save the URL encrypted in this workspace. Do not paste it into your conversation.</p>
      <button type="button" className="button" disabled={!name || !value} onClick={save}>Save destination securely</button>
    </> : <p>An administrator must save this workspace credential.</p>}
  </fieldset>;
}
