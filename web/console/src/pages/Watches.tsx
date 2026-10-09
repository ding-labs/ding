import { useState } from "react";
import {
  Link,
  useSearchParams,
  useParams,
  useNavigate,
  useLocation,
} from "react-router-dom";
import { Plus, Search, ArrowUpRight, Download, Pencil } from "lucide-react";
import { api, download } from "../api/client";
import type {
  StoreWatchPage,
  StoreWatchSummary,
  WatchrunStatus,
  StoreEventSummaryPage,
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
  Rule,
  Tabs,
  Raw,
} from "../components/common";
import { useDraft } from "../app/draft";
import { savedViews, savePreference } from "../app/preferences";
import { WatchActions } from "../components/Actions";
import { Events, Evidence } from "./Events";
import { Deliveries } from "./Deliveries";
export function Watches() {
  const location = useLocation();
  const [p, set] = useSearchParams();
  const query = useRead<StoreWatchPage>(`/console/watches?${p}`, 5000);
  const status = useRead<WatchrunStatus>("/status", 15000);
  const [saved, setSaved] = useState(0);
  const saveKey = `ding.views.${status.data?.instance}`;
  const views = savedViews(saveKey);
  const view = (key: string, value: string) => {
    const n = new URLSearchParams();
    if (value) n.set(key, value);
    set(n);
  };
  return (
    <>
      <Heading
        title="Every signal has a story."
        eyebrow="Watches"
        description="See what needs attention, then follow the evidence."
      >
        <Link className="button primary" to="/workbench">
          <Plus size={16} /> New watch
        </Link>
      </Heading>
      <div className="view-bar">
        <button
          className={!p.has("attention") && !p.has("status") ? "active" : ""}
          onClick={() => set({})}
        >
          All watches <b>{query.data?.all ?? "—"}</b>
        </button>
        <button
          className={p.has("attention") ? "active" : ""}
          onClick={() => view("attention", "yes")}
        >
          Needs attention <b>{query.data?.attention ?? "—"}</b>
        </button>
        <button
          className={p.get("status") === "paused" ? "active" : ""}
          onClick={() => view("status", "paused")}
        >
          Paused <b>{query.data?.paused ?? "—"}</b>
        </button>
      </div>
      <div className="filters">
        <label className="search">
          <Search size={16} />
          <input
            aria-label="Search watches"
            placeholder="Find a watch by name or ID…"
            value={p.get("search") || ""}
            onChange={(e) => {
              const n = new URLSearchParams(p);
              n.delete("cursor");
              n.set("search", e.target.value);
              set(n, { replace: true });
            }}
          />
        </label>
        <Filter
          name="source"
          label="Sources"
          options={["http", "push", "command"]}
        />
        <Filter
          name="status"
          label="Lifecycle"
          options={["running", "paused", "deleted"]}
        />
        <Filter name="incident" label="Incidents" options={["open", "none"]} />
        <Filter
          name="health"
          label="Source health"
          options={["error", "clear"]}
        />
        <Filter
          name="delivery"
          label="Delivery attention"
          options={["failed", "pending"]}
        />
      </div>
      <div className="view-preferences">
        <select
          aria-label="Saved view"
          value=""
          onChange={(e) => {
            if (e.target.value) set(new URLSearchParams(views[e.target.value]));
          }}
        >
          <option value="">Saved views</option>
          {Object.keys(views).map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
        <button
          className="text-button"
          disabled={!status.data}
          onClick={() => {
            const name = window.prompt("Name this view");
            if (name?.trim()) {
              const n = new URLSearchParams(p);
              n.delete("cursor");
              views[name.slice(0, 60)] = n.toString();
              savePreference(saveKey, JSON.stringify(views));
              setSaved(saved + 1);
            }
          }}
        >
          Save current view
        </button>
        <span className="muted">
          Snapshot <Time value={query.data?.at} />
        </span>
      </div>
      <ErrorBox
        error={query.error}
        retry={() => {
          if (p.has("cursor")) set({});
          else void query.refetch();
        }}
      />
      {query.isPending ? (
        <Loading />
      ) : (
        query.data && (
          <>
            {query.data.watches.length ? (
              <div className="table-wrap">
                <table>
                  <caption className="sr-only">
                    Watches and their current condition, source and delivery
                    state
                  </caption>
                  <thead>
                    <tr>
                      <th>Watch</th>
                      <th>Lifecycle</th>
                      <th>Condition</th>
                      <th>Source</th>
                      <th>Deliveries</th>
                      <th>Latest input</th>
                    </tr>
                  </thead>
                  <tbody>
                    {query.data.watches.map((w) => (
                      <tr key={w.id}>
                        <td>
                          <Link
                            className="row-title"
                            to={`/watches/${encodeURIComponent(w.id)}`}
                            state={{
                              returnTo: location.pathname + location.search,
                            }}
                          >
                            {w.name || w.id}
                            <ArrowUpRight size={14} />
                          </Link>
                          {w.name && <small>{w.id}</small>}
                          <small>
                            {w.source} · {w.trigger}
                          </small>
                        </td>
                        <td>
                          <Badge value={w.status} />
                        </td>
                        <td>
                          {!w.entities ? (
                            "Waiting for input"
                          ) : w.trigger === "transition" ? (
                            w.open ? (
                              <Badge value="firing">
                                {w.open} incident{w.open === 1 ? "" : "s"} open
                              </Badge>
                            ) : (
                              "No open incident"
                            )
                          ) : (
                            `${w.entities} evaluated entities`
                          )}
                        </td>
                        <td>
                          {w.unhealthy || w.lastError ? (
                            <Badge value="error">Acquisition error</Badge>
                          ) : w.missing.length ? (
                            <Badge value="warning">Missing credentials</Badge>
                          ) : (
                            <span>
                              {w.lastInputAt.startsWith("0001")
                                ? "Waiting"
                                : "Input received"}
                            </span>
                          )}
                          {w.missing.map((m) => (
                            <small key={m}>{m} absent</small>
                          ))}
                          {w.lastError && (
                            <small className="danger-text">{w.lastError}</small>
                          )}
                        </td>
                        <td>
                          {w.failed ? (
                            <Link
                              to={`/deliveries?watch=${encodeURIComponent(w.id)}`}
                            >
                              <Badge value="permanent">{w.failed} failed</Badge>
                            </Link>
                          ) : w.pending ? (
                            `${w.pending} queued / sending`
                          ) : (
                            "No delivery attention"
                          )}
                        </td>
                        <td>
                          <Time value={w.lastInputAt} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty
                title={
                  query.data.all
                    ? "No watches match this view"
                    : "Your next signal starts here."
                }
              >
                {query.data.all ? (
                  <button className="button" onClick={() => set({})}>
                    Clear filters
                  </button>
                ) : (
                  <>
                    <p>
                      Create an HTTP, push, or command watch. Understand and
                      test its rule before it runs.
                    </p>
                    <Link className="button primary" to="/workbench">
                      Create your first watch <Plus size={16} />
                    </Link>
                  </>
                )}
              </Empty>
            )}
            <Raw
              value={query.data}
              title="Watch summary response"
              name="ding-watches.json"
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
export function WatchDetail() {
  const draft = useDraft();
  const location = useLocation();
  const { id = "" } = useParams();
  const [p, set] = useSearchParams();
  const tab = p.get("tab") || "Overview";
  const navigate = useNavigate();
  const query = useRead<StoreWatchSummary>(
    `/watches/${encodeURIComponent(id)}?entitiesAfter=${encodeURIComponent(p.get("entitiesAfter") || "")}`,
    5000,
  );
  const summary = useRead<StoreWatchPage>(
    `/console/watches?watch=${encodeURIComponent(id)}&limit=1`,
    5000,
  );
  const latest = useRead<StoreEventSummaryPage>(
    `/console/events?watch=${encodeURIComponent(id)}&limit=1`,
    5000,
  );
  const [exportError, setExportError] = useState<unknown>();
  const exportIt = async (edit = false) => {
    try {
      const d = await api<{ manifest: string }>(
        `/watches/${encodeURIComponent(id)}/export`,
      );
      if (edit) {
        if (
          draft.dirty &&
          draft.manifest &&
          draft.manifest !== d.manifest &&
          !window.confirm(
            "Replace your unsaved Workbench manifest with this watch?",
          )
        )
          return;
        window.dispatchEvent(
          new CustomEvent("ding:draft", { detail: d.manifest }),
        );
        navigate("/workbench");
      } else download(`${id}.yaml`, d.manifest, "text/yaml");
    } catch (e) {
      setExportError(e);
    }
  };
  if (query.isPending) return <Loading />;
  if (!query.data)
    return <ErrorBox error={query.error} retry={() => void query.refetch()} />;
  const w = query.data.watch;
  const d = w.plan.definition;
  const s = summary.data?.watches[0];
  return (
    <>
      <Back to="/watches">Watches</Back>
      <Heading
        title={d.metadata.name || id}
        eyebrow={id}
        description={
          <>
            <Badge value={w.status} />{" "}
            <span className="mono">
              Revision {w.plan.revision.slice(0, 12)}
            </span>
          </>
        }
      >
        <button className="button" onClick={() => void exportIt()}>
          <Download size={15} />
          Export
        </button>
        <button className="button" onClick={() => void exportIt(true)}>
          <Pencil size={15} /> Edit in Workbench
        </button>
        <WatchActions watch={w} />
      </Heading>
      <ErrorBox error={query.error || exportError} />
      <Tabs
        items={["Overview", "Events", "Entities", "Deliveries", "Definition"]}
        active={tab}
        onChange={(t) => {
          const n = new URLSearchParams();
          n.set("tab", t);
          set(n, { state: location.state });
        }}
      />
      {tab === "Overview" && (
        <>
          <div className="overview-grid">
            <section className="panel rule-panel">
              <p className="eyebrow">The rule</p>
              <Rule definition={d} />
              <div className="rule-footer">
                {d.spec.source.type === "push" ? (
                  "Waiting for POST /v1/ingest/" + id
                ) : (
                  <>
                    Next scheduled check <Time value={w.nextAt} />
                  </>
                )}
              </div>
            </section>
            <section className="panel state-panel">
              <p className="eyebrow">Current state</p>
              <dl className="facts">
                <div>
                  <dt>Condition</dt>
                  <dd>
                    {!s?.entities
                      ? "Waiting for first input"
                      : d.spec.policy.trigger === "transition"
                        ? `${s.open} open incident${s.open === 1 ? "" : "s"} across ${s.entities} ${s.entities === 1 ? "entity" : "entities"}`
                        : `${s.entities} evaluated ${s.entities === 1 ? "entity" : "entities"}`}
                  </dd>
                </div>
                <div>
                  <dt>Source</dt>
                  <dd>
                    {s?.unhealthy || w.lastError
                      ? "Acquisition error"
                      : w.lastInputAt && !w.lastInputAt.startsWith("0001")
                        ? "Input received; freshness depends on rule"
                        : "No input yet"}
                  </dd>
                </div>
                <div>
                  <dt>Latest input</dt>
                  <dd>
                    <Time value={w.lastInputAt} />
                  </dd>
                </div>
                <div>
                  <dt>Deliveries</dt>
                  <dd>
                    {s?.failed || 0} failed · {s?.pending || 0} queued / sending
                  </dd>
                </div>
              </dl>
              {w.lastError && <p className="danger-text">{w.lastError}</p>}
              {s?.missing.length ? (
                <p className="notice">
                  Required credentials absent: {s.missing.join(", ")}
                </p>
              ) : null}
            </section>
          </div>
          <div className="section-heading">
            <div>
              <p className="eyebrow">Follow the evidence</p>
              <h2>Why the latest event happened</h2>
            </div>
            <Link to={`/events?watch=${encodeURIComponent(id)}`}>
              All events <ArrowUpRight size={14} />
            </Link>
          </div>
          {latest.data?.events[0] ? (
            <Evidence id={latest.data.events[0].id} compact />
          ) : (
            <Empty title="No recorded events yet">
              <p>
                The first event will appear here with its evidence. A watch can
                receive inputs without producing events.
              </p>
            </Empty>
          )}
          <section className="panel">
            <h3>Entity state</h3>
            {query.data.entities.slice(0, 5).map((e) => (
              <div className="entity-row" key={e.key}>
                <code>{e.key || "Default entity"}</code>
                <span>
                  {e.matches} matching · {e.recoveries} recovering · {e.samples}{" "}
                  window samples
                </span>
                <Badge
                  value={
                    e.sourceUnhealthy
                      ? "unknown"
                      : e.open
                        ? "firing"
                        : "neutral"
                  }
                >
                  {e.sourceUnhealthy
                    ? "Source unknown"
                    : e.open
                      ? "Incident open"
                      : "No open incident"}
                </Badge>
              </div>
            ))}
            {!query.data.entities.length && (
              <p className="muted">No evaluated entities yet.</p>
            )}
          </section>
        </>
      )}
      {tab === "Events" && <Events watch={id} />}
      {tab === "Deliveries" && <Deliveries watch={id} />}
      {tab === "Entities" && (
        <section className="panel">
          <h2>
            Entities <small>{s?.entities ?? ""} total</small>
          </h2>
          <p className="muted">
            Counters reflect the current checkpoint. An unknown source can
            retain an open incident.
          </p>
          {query.data.entities.map((e) => (
            <div className="entity-row" key={e.key}>
              <code>{e.key || "Default entity"}</code>
              <span>
                {e.matches} matches · {e.recoveries} recoveries · {e.samples}{" "}
                samples
              </span>
              <Badge value={e.open ? "firing" : "neutral"}>
                {e.open ? "Incident open" : "No open incident"}
              </Badge>
              <Badge value={e.sourceUnhealthy ? "unknown" : "neutral"}>
                {e.sourceUnhealthy ? "Source unhealthy" : "Source not flagged"}
              </Badge>
              <small>Input #{e.lastSequence}</small>
            </div>
          ))}
          {query.data.entitiesMore && (
            <button
              className="button"
              onClick={() => {
                const n = new URLSearchParams(p);
                n.set("entitiesAfter", query.data!.entitiesAfter);
                set(n, { state: location.state });
              }}
            >
              Next entities
            </button>
          )}
          {p.has("entitiesAfter") && (
            <button className="button" onClick={() => set({ tab: "Entities" })}>
              First entities
            </button>
          )}
        </section>
      )}
      {tab === "Overview" && d.spec.source.type === "push" && (
        <section className="panel">
          <h3>Send an observation</h3>
          <p>
            Use the daemon's ingest credential in your producer. The browser
            session cannot ingest. Replace the JSON body with the source data
            expected by this definition.
          </p>
          <pre className="command-block">{`curl --request POST '${window.location.origin}/v1/ingest/${encodeURIComponent(id)}' \\\n  --header "Authorization: Bearer $DING_INGEST_TOKEN" \\\n  --header 'Content-Type: application/json' \\\n  --data '{}'`}</pre>
          <p className="muted">
            Keep the ingest credential in your producer's secret store. Add an
            Idempotency-Key header when retrying an input.
          </p>
        </section>
      )}
      <Raw
        value={query.data}
        title="Watch inspection response"
        name={`${id}-inspection.json`}
      />
      {tab === "Definition" && (
        <section className="panel">
          <Rule definition={d} />
          <h3>Permissions</h3>
          {w.plan.permissions.map((x) => (
            <p key={x}>
              <code>{x}</code>
            </p>
          ))}
          <h3>Destination references</h3>
          {d.spec.destinations.map((x) => (
            <p key={x.ref}>
              <Link to="/system?tab=Destinations">{x.ref}</Link> ·{" "}
              {x.events.join(", ")}
            </p>
          ))}
          <p className="muted">
            Export includes current referenced destinations. Historical event
            evidence includes the definition from that event's revision.
          </p>
          <Raw value={w.plan} name={`${id}-definition.json`} />
        </section>
      )}
    </>
  );
}
