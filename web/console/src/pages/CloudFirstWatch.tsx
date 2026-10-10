import { useRef, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { api, APIError } from "../api/client";
import type { ControlFirstWatchPreview } from "../api/contracts";
import { Heading, ErrorBox, useRead } from "../components/common";

export function CloudFirstWatch() {
  const [id, setID] = useState("first-cloud-watch");
  const [url, setURL] = useState("");
  const [destination, setDestination] = useState("webhook");
  const [credential, setCredential] = useState("DING_WEBHOOK");
  const [value, setValue] = useState("");
  const [saved, setSaved] = useState(false);
  const [tested, setTested] = useState(false);
  const [sawTest, setSawTest] = useState(false);
  const [preview, setPreview] = useState<ControlFirstWatchPreview>();
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState<unknown>();
  const operation = useRef("");
  const names = useRead<{ names: string[] }>("/cloud/secrets");
  const available = saved || !!names.data?.names.includes(credential);
  const client = useQueryClient(); const navigate = useNavigate();

  function changed() { setPreview(undefined); setTested(false); setSawTest(false); operation.current = ""; setError(undefined); }
  async function save() {
    setBusy(true); setError(undefined);
    try { await api("/cloud/secrets", { method: "POST", body: { name: credential, value } }); setValue(""); setSaved(true); changed(); await client.invalidateQueries({ queryKey: ["/cloud/secrets"] }); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function review(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(undefined); setPreview(undefined);
    try { setPreview(await api<ControlFirstWatchPreview>("/onboarding/preview", { method: "POST", body: { id, url, destination, credential } })); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function testDelivery() {
    operation.current ||= crypto.randomUUID(); setBusy(true); setError(undefined);
    try {
      const result = await api<{ outcome: string }>("/cloud/destination-test", { method: "POST", body: { operationKey: operation.current, destination, credential } });
      if (result.outcome === "accepted") setTested(true);
      else if (result.outcome === "pending") setError(new Error("The test outcome is uncertain. Inspect your destination, then check this same test's status. A second notification will not be sent automatically."));
      else { operation.current = ""; setError(new Error("The destination did not confirm delivery. Inspect its URL, permissions and cloud usage before sending another test.")); }
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function activate() {
    if (!preview?.review.review || !sawTest) return;
    setBusy(true); setError(undefined);
    try {
      await api("/apply", { method: "POST", body: { manifest: preview.manifest, review: preview.review.review, dryRun: false } });
      await client.invalidateQueries(); navigate(`/watches/${encodeURIComponent(id)}`);
    } catch (e) { setError(e); if (e instanceof APIError && e.status === 409) setPreview(undefined); else if (!(e instanceof APIError) || e.status >= 500) setUncertain(true); }
    finally { setBusy(false); }
  }

  return <>
    <Heading eyebrow="Runs in Ding Cloud" title="Keep a useful watch running"
      description="Monitor a public service while your computer is off. Your local watches remain independent." />
    <section className="panel"><form className="onboarding-form" onSubmit={review}>
      <fieldset disabled={busy || uncertain}>
        <label>Watch ID<input required value={id} maxLength={64} onChange={e => { setID(e.target.value); changed(); }} /></label>
        <label>Public health endpoint<input required type="url" value={url} placeholder="https://your-service.example/health" onChange={e => { setURL(e.target.value); changed(); }} /></label>
        <p className="muted">The cloud beta supports public HTTP services. Localhost, private networks and command checks stay on your machine or your own server.</p>
        <label>Alert destination<select value={destination} onChange={e => { setDestination(e.target.value); changed(); }}>
          <option value="webhook">Webhook</option><option value="slack">Slack incoming webhook</option><option value="discord">Discord webhook</option>
        </select></label>
        <label>Credential reference<input required value={credential} pattern="[A-Za-z_][A-Za-z0-9_]*" maxLength={128} onChange={e => { setCredential(e.target.value); setSaved(false); changed(); }} /></label>
        {available ? <p className="muted">This credential is saved in your cloud workspace. Its value is never shown to a model.</p> : <>
          <label>Destination webhook URL<input type="password" autoComplete="new-password" value={value} onChange={e => setValue(e.target.value)} /></label>
          <p className="muted">Saving transfers this URL to Ding Cloud, encrypted in your workspace. Source configuration is sent when you preview.</p>
          <button className="button" type="button" disabled={!value || !credential} onClick={save}>Save destination securely</button>
        </>}
        <button className="button primary" type="submit" disabled={!available}>{busy ? "Working…" : "Preview cloud watch"}</button>
      </fieldset>
    </form></section>
    {error ? <ErrorBox error={error} /> : null}
    {uncertain ? <div className="notice" role="alert">The activation response was interrupted. Inspect <Link to={`/watches/${encodeURIComponent(id)}`}>this watch</Link> before retrying.</div> : null}
    {preview ? <section className="panel">
      <h2>Review hosted execution</h2>
      <p>Check every five minutes. Three consecutive HTTP 5xx responses fire; two nonmatching responses recover. Timeouts alert as source errors separately.</p>
      <p>Cloud limits include three watches, seven days of ordinary history, bounded response sizes and monthly network and delivery budgets. Pinned evidence counts against storage limits. <Link to="/system?tab=Cloud">Inspect usage and limits</Link>.</p>
      <button className="button" disabled={busy || uncertain || tested} onClick={testDelivery}>{tested ? "Test accepted by destination" : operation.current ? "Check test outcome" : "Send labeled delivery test"}</button>
      {tested ? <label><input type="checkbox" checked={sawTest} onChange={e => setSawTest(e.target.checked)} /> I received the test at my destination</label> : null}
      <details><summary>View exact manifest</summary><pre>{preview.manifest}</pre></details>
      <button className="button primary" disabled={busy || uncertain || !sawTest} onClick={activate}>Start in Ding Cloud</button>
    </section> : null}
  </>;
}
