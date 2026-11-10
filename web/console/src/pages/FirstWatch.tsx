import { useState, type FormEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { api, APIError, reviewedApply, unknownOutcome } from "../api/client";
import type { ControlFirstWatchPreview, StoreConsoleDestination } from "../api/contracts";
import { Heading, ErrorBox } from "../components/common";
import { NotificationSetup } from "../components/NotificationSetup";
import { Demo } from "../components/Demo";
import { FirstWatchRule } from "../components/FirstWatchRule";
import { SavedDestinationPicker } from "../components/SavedDestinationPicker";
import { SavedDestinationTest } from "../components/SavedDestinationTest";

export function FirstWatch() {
  const location = useLocation();
  const [id, setID] = useState(location.state?.anotherWatch ? "" : "first-watch");
  const [url, setURL] = useState("");
  const [delivery, setDelivery] = useState("desktop");
  const [savedDestination, setSavedDestination] = useState<StoreConsoleDestination>();
  const [savedConfirmed, setSavedConfirmed] = useState(false);
  const [preview, setPreview] = useState<ControlFirstWatchPreview>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [notification, setNotification] = useState(false);
  const [sawNotification, setSawNotification] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const navigate = useNavigate();
  const client = useQueryClient();

  const edit = (setter: (value: string) => void, value: string) => {
    setter(value); setPreview(undefined); setError(undefined); setUncertain(false); setSavedConfirmed(false); setSawNotification(false);
  };
  async function review(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(undefined); setPreview(undefined); setSavedConfirmed(false); setSawNotification(false);
    try {
      setPreview(await api<ControlFirstWatchPreview>("/onboarding/preview", {
        method: "POST", body: savedDestination ? { id, url, destinationRef: savedDestination.definition.metadata.id } : { id, url, delivery },
      }));
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function activate() {
    if (!preview?.review.review || (savedDestination ? !savedConfirmed : delivery === "desktop" && !sawNotification)) return;
    setBusy(true); setError(undefined);
    try {
      await reviewedApply({
        manifest: preview.manifest, review: preview.review.review, dryRun: false,
      });
      await client.invalidateQueries();
      navigate(`/watches/${encodeURIComponent(id)}`, { state: { firstWatch: id } });
    } catch (e) {
      setError(e);
      if (unknownOutcome(e)) setUncertain(true);
      else if (e instanceof APIError && e.status === 409) setPreview(undefined);
    } finally { setBusy(false); }
  }

  return <>
    <Heading eyebrow="Runs on this computer" title="Your first useful watch"
      description="Monitor a service you care about. No account, credit card, or AI connection needed."><Demo /></Heading>
    <section className="panel">
      <form className="onboarding-form" onSubmit={review}>
        <fieldset disabled={busy || uncertain}>
          <label>Watch ID<input required value={id} maxLength={64} pattern="[a-zA-Z0-9_-]+"
            onChange={e => edit(setID, e.target.value)} autoComplete="off" /></label>
          <label>Health endpoint<input required type="url" value={url}
            placeholder="http://localhost:3000/health" onChange={e => edit(setURL, e.target.value)} /></label>
          <p className="muted">Use a local development server or your deployed service. Put credentials in secret references through Workbench.</p>
          <SavedDestinationPicker value={savedDestination} onChange={value => { setSavedDestination(value); setPreview(undefined); setSavedConfirmed(false); setSawNotification(false); }} disabled={busy || uncertain} />
          {!savedDestination && <label>Alert delivery<select value={delivery} onChange={e => edit(setDelivery, e.target.value)}>
            <option value="desktop">Desktop notifications</option>
            <option value="console">Console only</option>
          </select></label>}
          <p className="muted">Configure new webhook, Slack, or Discord destinations in <Link to="/workbench" target="_blank" rel="noopener noreferrer">Workbench (new tab)</Link>, then refresh saved destinations here.</p>
          <button className="button primary" type="submit">{busy ? "Working…" : "Preview watch"}</button>
        </fieldset>
      </form>
    </section>
    {error ? <ErrorBox error={error} /> : null}
    {uncertain ? <div className="notice" role="alert">The activation response was interrupted. Check <Link to={`/watches/${encodeURIComponent(id)}`}>this watch</Link> before trying again.</div> : null}
    {preview ? <section className="panel">
      <h2>Review before starting</h2>
      <FirstWatchRule descriptions={preview.descriptions} />
      <p>This watch runs while this computer is awake and connected. Closing Ding's Console or your AI app does not stop the background service.</p>
      {savedDestination ? <SavedDestinationTest key={`${preview.review.review?.manifestHash}:${preview.review.review?.destinations[savedDestination.definition.metadata.id]}`} destinationRef={savedDestination.definition.metadata.id} revision={preview.review.review?.destinations[savedDestination.definition.metadata.id] || ""} endpoint="/onboarding/destination-test" disabled={busy || uncertain} onConfirmed={setSavedConfirmed} /> : delivery === "desktop" ? <>
        <NotificationSetup label="Send a test notification" disabled={busy || uncertain}
          onTest={() => { setNotification(false); setSawNotification(false); }}
          onAccepted={() => setNotification(true)} onUseConsole={() => edit(setDelivery, "console")} />
        {notification ? <label><input type="checkbox" checked={sawNotification} onChange={e => setSawNotification(e.target.checked)} /> I saw the test notification</label> : null}
        <p className="muted">Your OS may ask for notification permission. Acceptance by the OS does not guarantee you saw the alert.</p>
      </> : <p className="notice">Console-only delivery records alerts in Ding and the daemon output. It does not notify you outside Ding.</p>}
      <details><summary>View generated manifest</summary><pre>{preview.manifest}</pre></details>
      <button className="button primary" disabled={busy || uncertain || (savedDestination ? !savedConfirmed : delivery === "desktop" && !sawNotification)} onClick={activate}>Start this watch</button>
    </section> : null}
  </>;
}
