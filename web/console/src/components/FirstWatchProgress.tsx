import { Link } from "react-router-dom";
import type { StoreConsoleWatch, StoreLifecycleGuidance } from "../api/contracts";
import { watchActions } from "../app/watchLifecycle";
import { useCapability } from "../app/execution";
import { Time } from "./common";

export function FirstWatchProgress({ watch, watchID = watch?.id, status = watch?.status, lifecycle }: {
  watch?: StoreConsoleWatch;
  watchID?: string;
  status?: string;
  lifecycle?: StoreLifecycleGuidance | null;
}) {
  const manage = useCapability("manage");
  const resume = manage && watchActions(status ?? "", lifecycle).includes("resume");
  const received = Boolean(watch?.lastInputAt && !watch.lastInputAt.startsWith("0001"));
  const sourceError = Boolean(watch?.lastError || watch?.unhealthy || watch?.missing.length);
  const observed = received && Boolean(watch?.entities) && !sourceError;
  const running = status === "running";
  const title = status === "deleted" ? "Watch deleted"
    : status === "paused" ? "Monitoring is stopped"
    : !watch ? "Checking monitoring status…"
    : !running ? "Monitoring status unavailable"
    : sourceError ? "Monitoring needs attention"
    : observed ? "Monitoring input received"
    : "Waiting for the first observation";

  return <section className="panel" aria-labelledby="first-observation-title">
    <p className="eyebrow">Your watch is saved</p>
    <h2 id="first-observation-title" role="status">{title}</h2>
    {status === "deleted" ? <p>You can still inspect this watch’s recorded evidence and deliveries.</p>
      : lifecycle?.reason === "transfer_held" ? <p><a href="#watch-transfer">Inspect the transfer</a> before changing this watch.</p>
      : resume ? <p>Resume the watch to continue checking its source.</p>
      : !watch ? <p>Current monitoring status is unavailable. Saving the watch does not confirm a successful check.</p>
      : !running ? <p>Inspect the watch details and retained history below.</p>
      : sourceError ? <p>Check the source and credentials below. A source error does not prove your service is healthy or clear an open incident.</p>
      : observed ? <p>Ding received an observation at <Time value={watch.lastInputAt} />. This confirms monitoring input, not service health or notification delivery.</p>
      : <p>Ding has not confirmed monitoring input yet. Next scheduled check: <Time value={watch.nextAt} />. This view updates automatically.</p>}
    {watch && watch.failed > 0 && <p className="danger-text">A notification could not be delivered. Inspect delivery attempts before retrying.</p>}
    {watch && watch.pending > 0 && <p>Notifications are queued or being sent; receipt is not yet confirmed.</p>}
    {watchID && <div className="actions">
      {observed && running && <Link className="button primary" to="/start" state={{ anotherWatch: true }}>Create another watch</Link>}
      <Link className="button" to={`/watches/${encodeURIComponent(watchID)}?tab=Events`}>Inspect evidence</Link>
      <Link className="button" to={`/watches/${encodeURIComponent(watchID)}?tab=Deliveries`}>Inspect deliveries</Link>
    </div>}
  </section>;
}
