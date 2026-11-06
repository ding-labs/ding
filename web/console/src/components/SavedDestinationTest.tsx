import { useRef, useState } from "react";
import { api, APIError } from "../api/client";
import { useCapability } from "../app/execution";
import { ErrorBox } from "./common";

export function SavedDestinationTest({ destinationRef, revision, endpoint, disabled, onConfirmed }: {
  destinationRef: string;
  revision: string;
  endpoint: string;
  disabled?: boolean;
  onConfirmed: (confirmed: boolean) => void;
}) {
  const key = useRef("");
  const [busy, setBusy] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [error, setError] = useState<unknown>();
  const allowed = useCapability("destinations");
  async function test() {
    key.current ||= crypto.randomUUID();
    setBusy(true); setError(undefined); setConfirmed(false); onConfirmed(false);
    try {
      const result = await api<{ outcome: string }>(endpoint, { method: "POST", body: { operationKey: key.current, destinationRef, revision } });
      if (result.outcome === "accepted") setAccepted(true);
      else if (result.outcome === "pending") setError(new Error("The test outcome is uncertain. Check your destination before checking this same test's outcome. Ding will not send it again automatically."));
      else { key.current = ""; setError(new Error("The destination did not accept the test. Check its configuration before sending another labeled test.")); }
    } catch (err) {
      setError(err);
      if (err instanceof APIError && err.status === 409) setConflict(true);
    } finally { setBusy(false); }
  }
  return <>
    <ErrorBox error={error} />
    {!allowed && <p>An administrator must test notification delivery for this workspace.</p>}
    {conflict && <p>The destination changed. Preview the watch again before testing.</p>}
    <button className="button" type="button" disabled={disabled || busy || accepted || conflict || !allowed || !revision} onClick={test}>
      {accepted ? "Test accepted by destination" : key.current ? "Check saved destination test outcome" : "Send saved destination test"}
    </button>
    {accepted && <label><input type="checkbox" disabled={disabled} checked={confirmed} onChange={event => { setConfirmed(event.target.checked); onConfirmed(event.target.checked); }} /> I received the test at this destination</label>}
    <p className="muted">The test uses this saved destination's credentials and headers. Console tests appear in the daemon output. Acceptance alone does not confirm that you received a notification.</p>
  </>;
}
