import { useSearchParams, useParams, Link } from "react-router-dom";
import { RetryAction } from "../components/Actions";
import { useState } from "react";
import type {
  StoreDeliveryPage,
  StoreDeliveryInspection,
} from "../api/contracts";
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
  Raw,
} from "../components/common";
export function Deliveries({ watch }: { watch?: string }) {
  const [p, set] = useSearchParams();
  const q = new URLSearchParams();
  for (const k of ["watch", "status", "event", "destination", "cursor"])
    if (p.get(k)) q.set(k, p.get(k)!);
  if (watch) q.set("watch", watch);
  const query = useRead<StoreDeliveryPage>(`/console/deliveries?${q}`, 5000);
  return (
    <>
      {!watch && (
        <Heading
          title="Make every notification count."
          eyebrow="Deliveries"
          description="Follow the original event through attempts, retries, and receipt."
        />
      )}
      <div className="filters">
        <Filter
          name="status"
          label="Delivery states"
          options={[
            "pending",
            "leased",
            "delivered",
            "permanent",
            "exhausted",
            "canceled",
          ]}
        />
        {[...(!watch ? ["watch"] : []), "event", "destination"].map((k) => (
          <label className="filter grow" key={k}>
            <span>{k[0].toUpperCase() + k.slice(1)} ID</span>
            <input
              aria-label={`${k} ID`}
              placeholder={`All ${k}s`}
              value={p.get(k) || ""}
              onChange={(e) => {
                const n = new URLSearchParams(p);
                n.delete("cursor");
                n.set(k, e.target.value);
                set(n, { replace: true });
              }}
            />
          </label>
        ))}
      </div>
      <ErrorBox error={query.error} retry={() => void query.refetch()} />
      {query.isPending ? (
        <Loading />
      ) : (
        query.data && (
          <>
            {query.data.deliveries.length ? (
              <div className="table-wrap">
                <table>
                  <caption className="sr-only">Delivery intents</caption>
                  <thead>
                    <tr>
                      <th>Destination / event</th>
                      <th>Outcome</th>
                      <th>Watch</th>
                      <th>Attempts</th>
                      <th>Next attempt</th>
                    </tr>
                  </thead>
                  <tbody>
                    {query.data.deliveries.map((d) => (
                      <tr key={d.id}>
                        <td>
                          <Link
                            className="row-title"
                            to={`/deliveries/${d.id}`}
                          >
                            {d.destinationId}{" "}
                            <span className="muted">#{d.id}</span>
                          </Link>
                          <small>{d.eventId}</small>
                        </td>
                        <td>
                          <Badge value={d.status} />
                          {d.lastError && <small>{d.lastError}</small>}
                        </td>
                        <td>
                          <Link
                            to={`/watches/${encodeURIComponent(d.watchId)}`}
                          >
                            {d.watchId}
                          </Link>
                        </td>
                        <td>{d.attempts}</td>
                        <td>
                          {d.status === "pending" ? (
                            <Time value={d.nextAt} />
                          ) : (
                            "—"
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty title="No deliveries in this view">
                <p>
                  Delivery intents appear when a recorded event matches a
                  destination's event selection.
                </p>
              </Empty>
            )}
            <Raw
              value={query.data}
              title="Delivery summary response"
              name="ding-deliveries.json"
            />
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
export function DeliveryDetail() {
  const { id = "" } = useParams();
  const [before, setBefore] = useState(0);
  const query = useRead<StoreDeliveryInspection>(
    `/deliveries/${encodeURIComponent(id)}?before=${before}`,
    5000,
  );
  if (query.isPending) return <Loading />;
  if (!query.data)
    return <ErrorBox error={query.error} retry={() => void query.refetch()} />;
  const d = query.data;
  return (
    <>
      <Back to="/deliveries">Deliveries</Back>
      <Heading
        title={`Delivery to ${d.intent.destinationId}`}
        eyebrow={`Intent #${d.intent.id}`}
        description={<Badge value={d.intent.status} />}
      >
        <RetryAction intent={d.intent} />
      </Heading>
      <ErrorBox error={query.error} />
      <div className="overview-grid">
        <section className="panel">
          <h2>The original notification</h2>
          <dl className="facts">
            <div>
              <dt>Event</dt>
              <dd>
                <Link to={`/events/${encodeURIComponent(d.intent.eventId)}`}>
                  {d.intent.eventId}
                </Link>
              </dd>
            </div>
            <div>
              <dt>Watch</dt>
              <dd>
                <Link to={`/watches/${encodeURIComponent(d.intent.watchId)}`}>
                  {d.intent.watchId}
                </Link>
              </dd>
            </div>
            <div>
              <dt>Destination revision</dt>
              <dd>
                <code>{d.intent.destinationRevision}</code>
              </dd>
            </div>
            <div>
              <dt>Created</dt>
              <dd>
                <Time value={d.intent.createdAt} />
              </dd>
            </div>
            <div>
              <dt>Attempts in this cycle</dt>
              <dd>{d.intent.attempts}</dd>
            </div>
            <div>
              <dt>Next scheduled attempt</dt>
              <dd>
                {d.intent.status === "pending" ? (
                  <Time value={d.intent.nextAt} />
                ) : (
                  "None scheduled"
                )}
              </dd>
            </div>
          </dl>
          <Raw value={d.intent} title="Original intent and base64 payload" />
        </section>
        <section className="panel">
          <h2>Attempt history</h2>
          <p className="muted">
            Newest first. Details are recorded outcomes; successful receipt can
            be uncertain after a transport interruption.
          </p>
          {d.attempts.map((a, i) => (
            <article className="attempt" key={`${before}-${i}`}>
              <div className="actions">
                <Badge value={a.outcome} />
                <Time value={a.at} />
              </div>
              <p>{a.detail || "No additional detail recorded"}</p>
              <small>Attempt {a.attempt}</small>
            </article>
          ))}
          {!d.attempts.length && <p>No attempts recorded on this page.</p>}
          {d.attempts.length === 100 && (
            <button className="button" onClick={() => setBefore(d.before)}>
              Older attempts
            </button>
          )}
          {before > 0 && (
            <button className="button" onClick={() => setBefore(0)}>
              Newest attempts
            </button>
          )}
        </section>
      </div>
    </>
  );
}
