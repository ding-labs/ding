import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  ArrowRight,
  CheckCheck,
  Radio,
  Pause,
  Play,
  ArrowUpRight,
  Download,
} from "lucide-react";
import type {
  StoreEventSummaryPage,
  StoreEventPage,
  StoreObservationPage,
  StoreDeliveryPage,
  ReplayEvidence,
  WatchObservation,
} from "../api/contracts";
import { download } from "../api/client";
import {
  Heading,
  Badge,
  Time,
  useRead,
  Filter,
  Pager,
  ErrorBox,
  Loading,
  Empty,
  Back,
  Rule,
  Raw,
  labels,
} from "../components/common";
export function Events({ watch }: { watch?: string }) {
  const [p, set] = useSearchParams();
  const [following, follow] = useState(false);
  const [resume, setResume] = useState("");
  const [newCount, setNewCount] = useState(0);
  const q = new URLSearchParams();
  for (const k of ["watch", "type", "from", "to", "cursor"])
    if (p.get(k)) q.set(k, p.get(k)!);
  if (watch) q.set("watch", watch);
  const query = useRead<StoreEventSummaryPage>(`/console/events?${q}`);
  const selected = q.toString();
  useEffect(() => {
    setResume("");
    setNewCount(0);
  }, [selected]);
  const live = useRead<StoreEventPage>(
    `/events?watch=${encodeURIComponent(watch || p.get("watch") || "")}&cursor=${encodeURIComponent(resume || query.data?.followCursor || "")}&limit=100`,
    3000,
    following && !!query.data,
  );
  useEffect(() => {
    if (live.data) {
      setResume(live.data.cursor);
      setNewCount(
        (n) =>
          n +
          live.data.events.filter(
            (e) =>
              (!p.get("type") || e.type === p.get("type")) &&
              (!p.get("from") || e.at >= p.get("from")!) &&
              (!p.get("to") || e.at <= p.get("to")!),
          ).length,
      );
    }
  }, [live.data]);
  const reset = () => {
    const n = new URLSearchParams(p);
    n.delete("cursor");
    set(n);
    setResume("");
    setNewCount(0);
    void query.refetch();
  };
  return (
    <>
      {!watch && (
        <Heading
          title="The story, as it happened."
          eyebrow="Events"
          description="Recorded decisions, lifecycle changes, and the evidence behind them."
        />
      )}
      <div className="filters">
        {!watch && (
          <label className="filter grow">
            <span>Watch ID</span>
            <input
              aria-label="Filter by watch ID"
              value={p.get("watch") || ""}
              placeholder="All watches"
              onChange={(e) => {
                const n = new URLSearchParams(p);
                n.delete("cursor");
                n.set("watch", e.target.value);
                set(n, { replace: true });
              }}
            />
          </label>
        )}
        <Filter
          name="type"
          label="Event types"
          options={[
            "firing",
            "recovered",
            "changed",
            "new-event",
            "source_error",
            "source_recovered",
            "pause",
            "resume",
            "delete",
            "state_reset",
            "clock_reset",
            "gap",
          ]}
        />
        {["from", "to"].map((k) => (
          <label className="filter" key={k}>
            <span>
              {k === "from" ? "From (local time)" : "To (local time)"}
            </span>
            <input
              type="datetime-local"
              aria-label={k === "from" ? "Events from" : "Events to"}
              onChange={(e) => {
                const n = new URLSearchParams(p);
                n.delete("cursor");
                e.target.value
                  ? n.set(k, new Date(e.target.value).toISOString())
                  : n.delete(k);
                set(n);
              }}
            />
          </label>
        ))}
        <button
          className={following ? "button active" : "button"}
          onClick={() => follow(!following)}
        >
          {following ? <Pause size={15} /> : <Play size={15} />}{" "}
          {following ? "Pause live updates" : "Follow live"}
        </button>
      </div>
      {following && (
        <p className="subtle">
          <Radio size={13} /> Following new events. Pausing live updates does
          not pause watches.
        </p>
      )}
      {newCount > 0 && (
        <button className="new-events" onClick={reset}>
          {newCount} new event{newCount === 1 ? "" : "s"} · Show newest{" "}
          <ArrowRight size={14} />
        </button>
      )}
      <ErrorBox error={query.error || live.error} retry={reset} />
      {query.isPending ? (
        <Loading />
      ) : (
        query.data && (
          <>
            {query.data.events.length ? (
              <div className="timeline">
                {query.data.events.map((e, i) => {
                  const day = new Date(e.at).toLocaleDateString(undefined, {
                    weekday: "long",
                    month: "long",
                    day: "numeric",
                  });
                  const prev = query.data.events[i - 1];
                  return (
                    <div key={e.id}>
                      {(!prev ||
                        new Date(prev.at).toDateString() !==
                          new Date(e.at).toDateString()) && (
                        <div className="timeline-day">{day}</div>
                      )}
                      <Link
                        className="event-row"
                        to={`/events/${encodeURIComponent(e.id)}`}
                      >
                        <span
                          className={`event-symbol ${e.type === "firing" ? "firing" : ""}`}
                        >
                          <Radio size={17} />
                        </span>
                        <div className="event-copy">
                          <strong>
                            {labels[e.type] || e.type.replaceAll("_", " ")}
                          </strong>
                          <p>{e.message || e.type}</p>
                          <small>
                            {e.watchId}
                            {e.entity ? ` · ${e.entity}` : ""}
                          </small>
                        </div>
                        <div className="event-meta">
                          <Time value={e.at} />
                          <code>#{e.sequence}</code>
                        </div>
                        <ArrowUpRight size={15} />
                      </Link>
                    </div>
                  );
                })}
              </div>
            ) : (
              <Empty title="No retained events in this view">
                <p>
                  Events appear when a rule or lifecycle action produces a
                  recorded decision.
                </p>
              </Empty>
            )}
            <Pager
              more={query.data.more}
              cursor={query.data.cursor}
              total={query.data.total}
            />
          </>
        )
      )}
    </>
  );
}
export function EventDetail() {
  const { id = "" } = useParams();
  return (
    <>
      <Back to="/events">Events</Back>
      <Evidence id={id} />
    </>
  );
}
export function Evidence({
  id,
  compact = false,
}: {
  id: string;
  compact?: boolean;
}) {
  const proof = useRead<ReplayEvidence>(`/events/${encodeURIComponent(id)}`);
  const deliveries = useRead<StoreDeliveryPage>(
    `/console/deliveries?event=${encodeURIComponent(id)}`,
    5000,
  );
  const [after, setAfter] = useState(0);
  const [showInputs, setShowInputs] = useState(false);
  const inputs = useRead<StoreObservationPage>(
    `/events/${encodeURIComponent(id)}/observations?after=${after}`,
    false,
    showInputs,
  );
  if (proof.isPending) return <Loading />;
  if (!proof.data)
    return <ErrorBox error={proof.error} retry={() => void proof.refetch()} />;
  const d = proof.data;
  const c = d.checkpoint;
  const cond = d.definition.spec.condition;
  const eventName =
    d.event.type === "firing" &&
    d.definition.spec.policy.trigger === "transition"
      ? "Incident opened"
      : labels[d.event.type] || d.event.type;
  const intents = deliveries.data?.deliveries;
  const total = deliveries.data?.total;
  const failed =
    intents?.filter((x) => ["permanent", "exhausted"].includes(x.status))
      .length || 0;
  return (
    <section className={`evidence ${compact ? "compact" : ""}`}>
      <div className="evidence-heading">
        <div>
          <p className="eyebrow">
            {compact ? "Latest recorded event" : "Event evidence"} ·{" "}
            <Time value={d.event.at} />
          </p>
          <h2>{eventName}</h2>
          <p>{d.event.message}</p>
          <div className="subtle">
            <Link to={`/watches/${encodeURIComponent(d.event.watchId)}`}>
              {d.event.watchId}
            </Link>
            <span>Revision {d.event.revision.slice(0, 12)}</span>
            {d.event.entity && <code>{d.event.entity}</code>}
          </div>
        </div>
        {compact ? (
          <Link className="button" to={`/events/${encodeURIComponent(id)}`}>
            Inspect event <ArrowUpRight size={14} />
          </Link>
        ) : (
          <button
            className="button"
            onClick={() =>
              download(`${id}-evidence.json`, JSON.stringify(d, null, 2))
            }
          >
            <Download size={15} /> Download evidence
          </button>
        )}
      </div>
      <div className="evidence-strip">
        <a href="#observed">
          <span>01</span>
          <strong>Observed</strong>
          <small>{c ? c.input.health : "Lifecycle action"}</small>
        </a>
        <a href="#evaluated">
          <span>02</span>
          <strong>Evaluated</strong>
          <small>
            {c ? "Checkpoint available" : "No condition evaluation"}
          </small>
        </a>
        <a href="#recorded">
          <span>03</span>
          <strong>Recorded</strong>
          <small>Event #{d.event.sequence}</small>
        </a>
        <a href="#delivered">
          <span>04</span>
          <strong>Delivered</strong>
          <small>
            {deliveries.isError
              ? "Unavailable"
              : total === undefined
                ? "Reading…"
                : !total
                  ? "No intents recorded"
                  : failed
                    ? `${failed} failed on this page`
                    : intents?.every((x) => x.status === "delivered") &&
                        !deliveries.data?.more
                      ? "All delivered"
                      : "Inspect delivery states"}
          </small>
        </a>
      </div>
      <div className="evidence-body">
        <section id="observed">
          <p className="eyebrow">The observation</p>
          {c ? (
            <>
              <Observation observation={c.input} />
              {cond.field && c.input.fields && (
                <div className="value-card">
                  <span>{cond.field}</span>
                  <strong>
                    {typeof c.input.fields[cond.field] === "object"
                      ? JSON.stringify(c.input.fields[cond.field]).slice(0, 200)
                      : String(c.input.fields[cond.field] ?? "No value")}
                  </strong>
                  <small>Value in this input</small>
                </div>
              )}
              {c.input.deadline && (
                <p>
                  Logical deadline <Time value={c.input.deadline} />
                </p>
              )}
            </>
          ) : (
            <p className="muted">
              This event has no replay checkpoint. Lifecycle actions and older
              history may not include evaluated input.
            </p>
          )}
        </section>
        <section id="evaluated">
          <p className="eyebrow">The decision at this revision</p>
          <Rule definition={d.definition} />
          {c && (
            <>
              <div className="checkpoint">
                <div>
                  <span>Prior matching count</span>
                  <b>{c.prior.matches}</b>
                  <small>Checkpoint state</small>
                </div>
                <ArrowRight size={18} />
                <div>
                  <span>Trigger threshold</span>
                  <b>{d.definition.spec.policy.consecutive}</b>
                  <small>Consecutive matches</small>
                </div>
              </div>
              <p className="muted">
                Prior incident {c.prior.open ? "open" : "closed"}; recovery
                count {c.prior.recoveries}.{" "}
                {c.prior.sourceUnhealthy ? "Source was unhealthy." : ""}{" "}
                Historical counters are checkpoint state, not a reconstructed
                series of requests.
              </p>
              {cond.numeric && (
                <>
                  <p>
                    Window samples retained in checkpoint:{" "}
                    {c.prior.samples?.length || 0}. Gaps are preserved; values
                    are not interpolated.
                  </p>
                  {c.prior.overflowUntil && (
                    <p>
                      Overflow window ends{" "}
                      <Time value={c.prior.overflowUntil} />
                    </p>
                  )}
                  {c.prior.clockWindowUntil && (
                    <p>
                      Clock reset window ends{" "}
                      <Time value={c.prior.clockWindowUntil} />
                    </p>
                  )}
                  <Raw
                    value={c.prior.samples || []}
                    title="Window samples and boundaries"
                  />
                </>
              )}
              {cond.operator === "new-event" && (
                <p>
                  Deduplication horizon: {cond.dedupFor}. Checkpoint contains
                  the provider ID state needed for this decision.
                </p>
              )}
              {cond.missingFor && (
                <p>
                  Missing-input interval: {cond.missingFor}. Last logical input{" "}
                  <Time value={c.prior.lastAt} />; deadline{" "}
                  <Time value={c.prior.missingAt} />.
                </p>
              )}
            </>
          )}
          <Badge value={d.replayStatus === "verified" ? "verified" : "unknown"}>
            {d.replayStatus === "verified" ? (
              <>
                <CheckCheck size={13} /> Replay verified
              </>
            ) : (
              d.replayStatus
            )}
          </Badge>
        </section>
      </div>
      <div className="evidence-bottom">
        <section id="recorded">
          <h3>Recorded event</h3>
          <p>
            <code>{id}</code>
          </p>
          <Raw
            value={d}
            name={`${id}-evidence.json`}
            title="Event, definition and replay checkpoint"
          />
          <button
            className="text-button"
            onClick={() => setShowInputs(!showInputs)}
          >
            {showInputs ? "Hide" : "Inspect"} retained supporting observations
          </button>
          {showInputs && (
            <>
              <ErrorBox error={inputs.error} />
              {inputs.isPending ? (
                <Loading />
              ) : inputs.data?.observations.length ? (
                inputs.data.observations.map((o) => (
                  <Observation key={o.sequence} observation={o} />
                ))
              ) : (
                <p className="muted">
                  No supporting raw observations remain. The replay checkpoint
                  above may still preserve the input needed to verify this
                  event.
                </p>
              )}
              {inputs.data?.more && (
                <button
                  className="button"
                  onClick={() => setAfter(inputs.data!.after)}
                >
                  Next observations
                </button>
              )}
              {after > 0 && (
                <button className="button" onClick={() => setAfter(0)}>
                  First observations
                </button>
              )}
            </>
          )}
        </section>
        <section id="delivered">
          <h3>What happened next</h3>
          <ErrorBox error={deliveries.error} />
          {!total ? (
            <p className="muted">
              {deliveries.isPending
                ? "Reading delivery history…"
                : "No delivery intents were recorded for this event."}
            </p>
          ) : (
            <>
              {intents?.map((x) => (
                <Link
                  className="delivery-link"
                  key={x.id}
                  to={`/deliveries/${x.id}`}
                >
                  <div>
                    <strong>{x.destinationId}</strong>
                    <small>
                      {x.attempts} attempts · revision{" "}
                      {x.destinationRevision.slice(0, 8)}
                    </small>
                  </div>
                  <Badge value={x.status} />
                  <ArrowUpRight size={14} />
                </Link>
              ))}
              {deliveries.data?.more && (
                <Link to={`/deliveries?event=${encodeURIComponent(id)}`}>
                  View all {total} deliveries
                </Link>
              )}
            </>
          )}
        </section>
      </div>
    </section>
  );
}
function Observation({ observation: o }: { observation: WatchObservation }) {
  return (
    <div className="observation">
      <div className="actions">
        <code>Input #{o.sequence}</code>
        <Badge value={o.health} />
      </div>
      <dl className="facts">
        <div>
          <dt>Accepted</dt>
          <dd>
            <Time value={o.acceptedAt} />
          </dd>
        </div>
        {o.observedAt && (
          <div>
            <dt>Source timestamp</dt>
            <dd>
              <Time value={o.observedAt} />
            </dd>
          </div>
        )}
      </dl>
      {o.detail && <p>{o.detail}</p>}
      <Raw
        value={o}
        title="Observation fields"
        name={`observation-${o.sequence}.json`}
      />
    </div>
  );
}
