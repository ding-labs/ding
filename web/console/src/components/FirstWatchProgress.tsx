import { Link } from "react-router-dom";
import type { StoreConsoleWatch } from "../api/contracts";
import { Time } from "./common";

export function FirstWatchProgress({ watch }: { watch?: StoreConsoleWatch }) {
  const received = Boolean(watch?.lastInputAt && !watch.lastInputAt.startsWith("0001"));
  const sourceError = Boolean(watch?.lastError || watch?.unhealthy || watch?.missing.length);
  const observed = received && Boolean(watch?.entities) && !sourceError;
  const running = watch?.status === "running";
  const title = !watch ? "Checking monitoring status…"
    : watch.status === "deleted" ? "Watch deleted"
    : !running ? "Monitoring is stopped"
    : sourceError ? "Monitoring needs attention"
    : observed ? "Monitoring input received"
    : "Waiting for the first observation";

  return <section className="panel" aria-labelledby="first-observation-title">
    <p className="eyebrow">Your watch is saved</p>
    <h2 id="first-observation-title" role="status">{title}</h2>
    {!watch ? <p>Current monitoring status is unavailable. Saving the watch does not confirm a successful check.</p>
      : watch.status === "deleted" ? <p>You can still inspect this watch’s recorded evidence and deliveries.</p>
      : !running ? <p>Resume the watch to continue checking its source.</p>
      : sourceError ? <p>Check the source and credentials below. A source error does not prove your service is healthy or clear an open incident.</p>
      : observed ? <p>Ding received an observation at <Time value={watch.lastInputAt} />. This confirms monitoring input, not service health or notification delivery.</p>
      : <p>Ding has not confirmed monitoring input yet. Next scheduled check: <Time value={watch.nextAt} />. This view updates automatically.</p>}
    {watch && watch.failed > 0 && <p className="danger-text">A notification could not be delivered. Inspect delivery attempts before retrying.</p>}
    {watch && watch.pending > 0 && <p>Notifications are queued or being sent; receipt is not yet confirmed.</p>}
    {watch && <div className="actions">
      {observed && running && <Link className="button primary" to="/start" state={{ anotherWatch: true }}>Create another watch</Link>}
      <Link className="button" to={`/watches/${encodeURIComponent(watch.id)}?tab=Events`}>Inspect evidence</Link>
      <Link className="button" to={`/watches/${encodeURIComponent(watch.id)}?tab=Deliveries`}>Inspect deliveries</Link>
    </div>}
  </section>;
}
