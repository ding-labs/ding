import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { api, APIError } from "../api/client";
import type { ControlFirstWatchPreview } from "../api/contracts";
import { Heading, ErrorBox } from "../components/common";

export function FirstWatch() {
  const [id, setID] = useState("first-watch");
  const [url, setURL] = useState("");
  const [delivery, setDelivery] = useState("desktop");
  const [preview, setPreview] = useState<ControlFirstWatchPreview>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [notification, setNotification] = useState(false);
  const [sawNotification, setSawNotification] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const navigate = useNavigate();
  const client = useQueryClient();

  const edit = (setter: (value: string) => void, value: string) => {
    setter(value); setPreview(undefined); setError(undefined); setUncertain(false);
  };
  async function review(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(undefined); setPreview(undefined);
    try {
      setPreview(await api<ControlFirstWatchPreview>("/onboarding/preview", {
        method: "POST", body: { id, url, delivery },
      }));
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function testNotification() {
    setBusy(true); setError(undefined); setNotification(false); setSawNotification(false);
    try {
      await api("/desktop/test", { method: "POST", body: {} });
      setNotification(true);
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function activate() {
    if (!preview?.review.review) return;
    setBusy(true); setError(undefined);
    try {
      await api("/apply", { method: "POST", body: {
        manifest: preview.manifest, review: preview.review.review, dryRun: false,
      } });
      await client.invalidateQueries();
      navigate(`/watches/${encodeURIComponent(id)}`);
    } catch (e) {
      setError(e);
      if (e instanceof APIError && e.status === 409) setPreview(undefined);
      else if (!(e instanceof APIError) || e.status >= 500) setUncertain(true);
    } finally { setBusy(false); }
  }

  return <>
    <Heading eyebrow="Runs on this computer" title="Your first useful watch"
      description="Monitor a service you care about. No account, credit card, or AI connection needed." />
    <section className="panel">
      <form className="onboarding-form" onSubmit={review}>
        <fieldset disabled={busy || uncertain}>
          <label>Watch ID<input required value={id} maxLength={64} pattern="[a-zA-Z0-9_-]+"
            onChange={e => edit(setID, e.target.value)} autoComplete="off" /></label>
          <label>Health endpoint<input required type="url" value={url}
            placeholder="http://localhost:3000/health" onChange={e => edit(setURL, e.target.value)} /></label>
          <p className="muted">Use a local development server or your deployed service. Put credentials in secret references through Workbench.</p>
          <label>Alert delivery<select value={delivery} onChange={e => edit(setDelivery, e.target.value)}>
            <option value="desktop">Desktop notifications</option>
            <option value="console">Console only</option>
          </select></label>
          <p className="muted">For webhook, Slack, or Discord delivery, use <Link to="/workbench">Workbench</Link>.</p>
          <button className="button primary" type="submit">{busy ? "Working…" : "Preview watch"}</button>
        </fieldset>
      </form>
    </section>
    {error ? <ErrorBox error={error} /> : null}
    {uncertain ? <div className="notice" role="alert">The activation response was interrupted. Check <Link to={`/watches/${encodeURIComponent(id)}`}>this watch</Link> before trying again.</div> : null}
    {preview ? <section className="panel">
      <h2>Review before starting</h2>
      <p>Check every 30 seconds. Alert after three consecutive HTTP responses with a status of 500 or above. Recover after two nonmatching responses.</p>
      <p>Timeouts and unreachable endpoints produce source-error alerts separately. They never count as successful checks.</p>
      <p>This watch runs while this computer is awake and connected. Closing Ding's Console or your AI app does not stop the background service.</p>
      {delivery === "desktop" ? <>
        <button className="button" disabled={busy || uncertain} onClick={testNotification}>Send a test notification</button>
        {notification ? <label><input type="checkbox" checked={sawNotification} onChange={e => setSawNotification(e.target.checked)} /> I saw the test notification</label> : null}
        <p className="muted">Your OS may ask for notification permission. Acceptance by the OS does not guarantee you saw the alert.</p>
      </> : <p className="notice">Console-only delivery records alerts in Ding and the daemon output. It does not notify you outside Ding.</p>}
      <details><summary>View generated manifest</summary><pre>{preview.manifest}</pre></details>
      <button className="button primary" disabled={busy || uncertain || (delivery === "desktop" && !sawNotification)} onClick={activate}>Start this watch</button>
    </section> : null}
  </>;
}
