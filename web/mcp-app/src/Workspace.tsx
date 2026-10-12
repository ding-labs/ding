import { useEffect, useRef, useState } from "react";
import bell from "../../../design/assets/mark.svg?raw";
import type { Bridge, DingView, ToolResult } from "./bridge";
import { parseResult } from "./bridge";
import type {
  StoreConsoleWatch,
  StoreEventSummary,
  StoreConsoleDelivery,
  StoreWatchSummary,
  ReplayEvidence,
  WatchrunApplyResult,
  WatchrunChange,
} from "../../console/src/api/contracts";

type Pending = { name: string; args: Record<string, unknown> };
type Preview = {
  valid: boolean;
  preview?: { handle: string; expiresAt: string; changes: WatchrunApplyResult };
  descriptions?: {
    id: string;
    condition: string;
    policy: string;
    source: string;
  }[];
  diagnostics?: { message: string }[];
  fixture?: { observations: number; eventCount: number };
};

function date(value: string) {
  const d = new Date(value);
  return Number.isNaN(d.valueOf())
    ? value
    : new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(d);
}
function Badge({ value }: { value: string }) {
  return (
    <span
      className={`badge ${["running", "delivered", "verified", "created"].includes(value) ? "good" : ["permanent", "exhausted", "deleted", "reset"].includes(value) ? "warn" : ""}`}
    >
      <i />
      {value.replaceAll("_", " ")}
    </span>
  );
}
function Details({ title, value }: { title: string; value: unknown }) {
  return (
    <details>
      <summary>{title}</summary>
      <pre>
        {typeof value === "string" ? value : JSON.stringify(value, null, 2)}
      </pre>
    </details>
  );
}
function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div className="empty">
      <div className="empty-ring">✓</div>
      {children}
    </div>
  );
}

export function Workspace({ bridge }: { bridge: Bridge }) {
  const [view, setView] = useState<DingView>();
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [scopes, setScopes] = useState<string[]>([]);
  const [location, setLocation] = useState("your Ding");
  const [cloud, setCloud] = useState(false);
  const workspaceRef = useRef<string | undefined>(undefined);
  const [workspaceID, setWorkspaceID] = useState("");
  const [workspaces, setWorkspaces] = useState<{ id: string; name: string }[]>(
    [],
  );
  const connectionRef = useRef<string | undefined>(undefined);
  const executionRef = useRef<string | undefined>(undefined);
  const [connectionID, setConnectionID] = useState("");
  const [connections, setConnections] = useState<
    { id: string; name: string }[]
  >([]);
  function targetArgs(id = workspaceRef.current) {
    return {
      ...(connectionRef.current
        ? { connection_id: connectionRef.current }
        : {}),
      ...(id ? { workspace_id: id } : {}),
    };
  }
  const [pending, setPending] = useState<Pending>();
  const pendingRef = useRef<Pending | undefined>(undefined);
  const connected = useRef(false);
  const inFlight = useRef(false);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState<Record<string, unknown>>({});
  const canManage = scopes.includes("manage");

  function receive(result: ToolResult) {
    try {
      const next = parseResult(result);
      if (next.target) {
        if (
          pendingRef.current &&
          (next.target.workspace !== pendingRef.current.args.workspace_id ||
            next.target.connection !== pendingRef.current.args.connection_id)
        ) {
          setError(
            "Resolve the pending change before viewing another workspace.",
          );
          return;
        }
        const changed =
          workspaceRef.current &&
          (workspaceRef.current !== next.target.workspace ||
            connectionRef.current !== next.target.connection);
        if (changed) {
          setScopes([]);
          setCloud(false);
        }
        connectionRef.current = next.target.connection;
        setConnectionID(next.target.connection ?? "");
        workspaceRef.current = next.target.workspace;
        setWorkspaceID(next.target.workspace);
        if (changed)
          void loadCapabilities(next.target.workspace).catch((e) =>
            setError(String(e.message)),
          );
      }
      setView(next);
      setQuery(next.query ?? {});
      setSearch(String(next.query?.search ?? ""));
      setError("");
    } catch (e) {
      setError(String((e as Error).message));
    }
  }
  async function loadCapabilities(id = workspaceRef.current) {
    const connection = connectionRef.current;
    const result = parseResult(
      await bridge.call("ding_get_capabilities", targetArgs(id)),
    );
    if (
      connection !== connectionRef.current ||
      (id && workspaceRef.current && id !== workspaceRef.current)
    )
      return;
    const grant = result.data.grant as { scopes?: string[] };
    setScopes(
      Array.isArray(result.data.effectiveScopes)
        ? (result.data.effectiveScopes as string[])
        : (grant.scopes ?? []),
    );
    const execution = result.data.execution;
    executionRef.current = String(execution);
    setCloud(execution === "cloud");
    const workspace =
      typeof result.data.workspaceName === "string"
        ? result.data.workspaceName
        : "";
    setLocation(
      execution === "cloud"
        ? `Ding Cloud${workspace ? ` · ${workspace}` : ""}`
        : execution === "shared"
          ? `your server${workspace ? ` · ${workspace}` : ""}`
          : "this computer",
    );
    if (result.target) {
      connectionRef.current = result.target.connection;
      setConnectionID(result.target.connection ?? "");
      workspaceRef.current = result.target.workspace;
      setWorkspaceID(result.target.workspace);
    }
  }
  async function loadWorkspaces() {
    const result = parseResult(
      await bridge.call(
        "ding_list_workspaces",
        connectionRef.current ? { connection_id: connectionRef.current } : {},
      ),
    );
    if (!Array.isArray(result.data.workspaces))
      throw new Error("Workspace discovery is unavailable.");
    setWorkspaces(result.data.workspaces as { id: string; name: string }[]);
  }
  async function selectWorkspace(id: string) {
    if (!id || pendingRef.current || inFlight.current) return;
    inFlight.current = true;
    workspaceRef.current = id;
    setWorkspaceID(id);
    setBusy(true);
    setScopes([]);
    setView(undefined);
    try {
      await loadCapabilities(id);
      receive(await bridge.call("ding_list_watches", targetArgs(id)));
    } catch (e) {
      setError(String((e as Error).message));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  async function loadConnections() {
    const result = parseResult(await bridge.call("ding_list_connections", {}));
    if (!Array.isArray(result.data.connections))
      throw new Error("Connection discovery is unavailable.");
    setConnections(result.data.connections as { id: string; name: string }[]);
  }
  async function selectConnection(id: string) {
    if (!id || pendingRef.current || inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    connectionRef.current = id;
    setConnectionID(id);
    workspaceRef.current = undefined;
    setWorkspaceID("");
    setWorkspaces([]);
    setScopes([]);
    setCloud(false);
    setView(undefined);
    setLocation("the selected connection");
    try {
      await loadCapabilities();
      if (executionRef.current !== "local") await loadWorkspaces();
      receive(await bridge.call("ding_list_watches", targetArgs()));
    } catch (e) {
      setError(String((e as Error).message));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  useEffect(() => {
    if (connected.current) return;
    connected.current = true;
    bridge
      .connect(receive)
      .then(async () => {
        try {
          await loadCapabilities();
          if (connectionRef.current) await loadConnections();
          if (workspaceRef.current && executionRef.current !== "local")
            await loadWorkspaces();
        } catch (e) {
          if (String((e as Error).message).includes("connection_required")) {
            await loadConnections();
            setError("Choose a connection to continue.");
          } else if (
            String((e as Error).message).includes("workspace_required")
          ) {
            await loadWorkspaces();
            setError("Choose a workspace to continue.");
          } else throw e;
        }
        setReady(true);
      })
      .catch((e) => setError(String((e as Error).message)));
  }, [bridge]);

  async function call(
    name: string,
    args: Record<string, unknown> = {},
    mutation = false,
    retry?: Pending,
  ) {
    if (inFlight.current || !ready) return;
    args = { ...targetArgs(), ...args };
    if (mutation && !retry && pendingRef.current) {
      const { operation_key: _key, ...priorArgs } = pendingRef.current.args;
      if (
        pendingRef.current.name !== name ||
        JSON.stringify(priorArgs) !== JSON.stringify(args)
      ) {
        setError(
          "Check the saved result of the previous action before starting another change.",
        );
        return;
      }
      retry = pendingRef.current;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    const request = retry ?? {
      name,
      args: mutation ? { ...args, operation_key: crypto.randomUUID() } : args,
    };
    if (mutation) {
      pendingRef.current = request;
      setPending(request);
    }
    try {
      const result = parseResult(await bridge.call(request.name, request.args));
      setView(result);
      if (result.query) setQuery(result.query);
      if (
        mutation ||
        (request.name === "ding_get_operation" &&
          request.args.operation_key === pendingRef.current?.args.operation_key)
      ) {
        pendingRef.current = undefined;
        setPending(undefined);
      }
    } catch (e) {
      const message = String((e as Error).message);
      setError(message);
      if (/(?:^|: )(integration_denied|permission_denied):/.test(message))
        setScopes([]);
      if (
        mutation &&
        /(?:^|: )(integration_denied|permission_denied|workspace_rate|revision_conflict|preview_expired|operation_conflict|invalid_request|integration_limit|quota_exceeded|shutting_down|integration_failed):/.test(
          message,
        )
      ) {
        pendingRef.current = undefined;
        setPending(undefined);
      }
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  function navigate(kind: string) {
    setQuery({});
    void call(`ding_list_${kind}`);
  }
  function next(kind: string, cursor: unknown) {
    void call(`ding_list_${kind}`, { ...query, cursor });
  }

  const data = view?.data;
  const watches = (data?.watches ?? []) as StoreConsoleWatch[];
  const events = (data?.events ?? []) as StoreEventSummary[];
  const deliveries = (data?.deliveries ?? []) as StoreConsoleDelivery[];
  const preview = data as Preview | undefined;
  const watch =
    view?.view === "watch" ? (data as unknown as StoreWatchSummary) : undefined;
  const evidence =
    view?.view === "evidence" ? (data as unknown as ReplayEvidence) : undefined;
  const changes =
    view?.view === "applied"
      ? (data as unknown as WatchrunApplyResult)
      : preview?.preview?.changes;
  const title: Record<string, string> = {
    watches: "A little peace of mind.",
    events: "What changed.",
    deliveries: "Every notification, accounted for.",
    watch: "Watch details",
    preview: "Review your changes",
    applied: "Your changes are live.",
    evidence: "The story behind this event",
    lifecycle: "Watch updated",
    delivery: "Delivery details",
    cloud_status: "Cloud usage and health",
  };

  return (
    <main aria-busy={busy}>
      <header>
        <div className="brand">
          {/* Trusted artwork bundled at build time; no remote image requests. */}
          <span
            className="brand-icon"
            aria-hidden="true"
            dangerouslySetInnerHTML={{ __html: bell }}
          />
          <strong>Ding</strong>
        </div>
        <span className="connection">
          <i />
          {ready ? `Connected to ${location}` : "Connecting…"}
        </span>
      </header>
      {connections.length > 0 && (
        <label>
          Connection
          <select
            value={connectionID}
            disabled={busy || Boolean(pending)}
            onChange={(event) => void selectConnection(event.target.value)}
          >
            <option value="" disabled>
              Choose a connection
            </option>
            {connections.map((connection) => (
              <option key={connection.id} value={connection.id}>
                {connection.name}
              </option>
            ))}
          </select>
        </label>
      )}
      {workspaces.length > 0 && (
        <label>
          Workspace
          <select
            value={workspaceID}
            disabled={busy || Boolean(pending)}
            onChange={(event) => void selectWorkspace(event.target.value)}
          >
            <option value="" disabled>
              Choose a workspace
            </option>
            {workspaces.map((workspace) => (
              <option key={workspace.id} value={workspace.id}>
                {workspace.name}
              </option>
            ))}
          </select>
        </label>
      )}
      <nav aria-label="Ding views">
        {["watches", "events", "deliveries"].map((kind) => (
          <button
            key={kind}
            disabled={
              !ready ||
              busy ||
              (workspaces.length > 0 && !workspaceID) ||
              (connections.length > 0 && !connectionID)
            }
            aria-current={view?.view === kind ? "page" : undefined}
            onClick={() => navigate(kind)}
          >
            {kind[0].toUpperCase() + kind.slice(1)}
          </button>
        ))}
        {cloud && (
          <button
            disabled={!ready || busy}
            aria-current={view?.view === "cloud_status" ? "page" : undefined}
            onClick={() => void call("ding_get_cloud_status")}
          >
            Cloud usage
          </button>
        )}
      </nav>
      {error && (
        <div className="error" role="alert">
          <strong>Something needs attention.</strong>
          <p>{error}</p>
          {pending && (
            <div className="actions">
              <button
                disabled={busy}
                onClick={() =>
                  void call("ding_get_operation", {
                    operation_key: pending.args.operation_key,
                  })
                }
              >
                Check saved result
              </button>
              <button
                disabled={busy}
                onClick={() =>
                  void call(pending.name, pending.args, true, pending)
                }
              >
                Retry same request
              </button>
            </div>
          )}
        </div>
      )}
      {!view ? (
        <Empty>
          <h1>Your watches, close at hand.</h1>
          <p>Ding keeps watch while you get on with your day.</p>
          <button
            className="primary"
            disabled={
              !ready ||
              busy ||
              (workspaces.length > 0 && !workspaceID) ||
              (connections.length > 0 && !connectionID)
            }
            onClick={() => navigate("watches")}
          >
            See my watches
          </button>
        </Empty>
      ) : (
        <>
          <div className="heading">
            <span className="eyebrow">
              {view.view === "preview"
                ? "BEFORE ANYTHING CHANGES"
                : "YOUR DING"}
            </span>
            <h1>{title[view.view] ?? "Ding activity"}</h1>
          </div>
          {view.view === "cloud_status" && <CloudStatus data={view.data} />}
          {view.view === "watches" && (
            <>
              <div className="metrics">
                <div>
                  <b>{Number(data?.all ?? data?.total ?? 0)}</b>
                  <span>Watches</span>
                </div>
                <div>
                  <b>{Number(data?.attention ?? 0)}</b>
                  <span>Need attention</span>
                </div>
                <div>
                  <b>{Number(data?.paused ?? 0)}</b>
                  <span>Paused</span>
                </div>
              </div>
              <form
                className="search"
                onSubmit={(e) => {
                  e.preventDefault();
                  const q = { search };
                  setQuery(q);
                  void call("ding_list_watches", q);
                }}
              >
                <label className="sr-only" htmlFor="search">
                  Search watches
                </label>
                <input
                  id="search"
                  placeholder="Find a watch…"
                  value={search}
                  maxLength={200}
                  onChange={(e) => setSearch(e.target.value)}
                />
                <button disabled={busy}>Search</button>
              </form>
              {!watches.length ? (
                <Empty>
                  <h2>
                    {search
                      ? "No matching watches"
                      : "Nothing to keep an eye on yet."}
                  </h2>
                  <p>
                    Ask Ding to watch a service, a JSON endpoint, or an incoming
                    event.
                  </p>
                </Empty>
              ) : (
                <div className="list">
                  {watches.map((w) => (
                    <button
                      className="watch-row"
                      key={w.id}
                      disabled={busy}
                      onClick={() =>
                        void call("ding_get_watch", { watch_id: w.id })
                      }
                    >
                      <span
                        className={`status-dot ${w.status === "running" ? "on" : ""}`}
                      />
                      <span className="row-main">
                        <strong>{w.name || w.id}</strong>
                        <small>
                          {w.source} ·{" "}
                          {w.open
                            ? `${w.open} open incident${w.open === 1 ? "" : "s"}`
                            : w.unhealthy
                              ? "Source needs attention"
                              : w.failed
                                ? "Notification needs attention"
                                : "No open incidents"}
                        </small>
                      </span>
                      <Badge value={w.status} />
                      <span aria-hidden="true">↗</span>
                    </button>
                  ))}
                </div>
              )}
              {Boolean(data?.more) && (
                <button
                  className="more"
                  disabled={busy}
                  onClick={() => next("watches", data?.cursor)}
                >
                  Next page
                </button>
              )}
            </>
          )}
          {watch && (
            <section>
              <div className="card">
                <div className="card-title">
                  <h2>
                    {watch.watch.plan.definition.metadata.name ||
                      watch.watch.plan.definition.metadata.id}
                  </h2>
                  <Badge value={watch.watch.status} />
                </div>
                <p>Source: {watch.watch.plan.definition.spec.source.type}</p>
                <div className="actions">
                  {watch.watch.status !== "deleted" && (
                    <button
                      className="primary"
                      disabled={!canManage || busy}
                      onClick={() =>
                        void call(
                          watch.watch.status === "paused"
                            ? "ding_resume_watch"
                            : "ding_pause_watch",
                          {
                            watch_id: watch.watch.plan.definition.metadata.id,
                            expected_revision: watch.watch.plan.revision,
                            expected_generation: watch.watch.generation,
                          },
                          true,
                        )
                      }
                    >
                      {watch.watch.status === "paused"
                        ? "Resume watch"
                        : "Pause watch"}
                    </button>
                  )}
                  <button
                    disabled={busy}
                    onClick={() => {
                      const q = {
                        watch: watch.watch.plan.definition.metadata.id,
                      };
                      setQuery(q);
                      void call("ding_list_events", q);
                    }}
                  >
                    See events
                  </button>
                </div>
                {!canManage && (
                  <p className="muted">
                    This connection has view access. Review management
                    permission in this instance’s model connection settings.
                  </p>
                )}
                <Details
                  title="Watch definition"
                  value={watch.watch.plan.definition}
                />
              </div>
              <Details
                title="Current entities and recent deliveries"
                value={{
                  entities: watch.entities,
                  deliveries: watch.deliveries,
                }}
              />
              {(watch.entitiesMore || watch.deliveriesMore) && (
                <p className="muted">
                  Showing a bounded snapshot. Ask for delivery history to
                  inspect further.
                </p>
              )}
            </section>
          )}
          {(view.view === "preview" || view.view === "applied") && (
            <section>
              {preview?.valid === false && (
                <div className="error">
                  {preview.diagnostics?.map((d, i) => (
                    <p key={i}>{d.message}</p>
                  ))}
                </div>
              )}
              {preview?.descriptions?.map((d) => (
                <div className="card" key={d.id}>
                  <h2>{d.id}</h2>
                  <p>{d.condition}</p>
                  <p className="muted">
                    {d.source} · {d.policy}
                  </p>
                </div>
              ))}
              {changes && (
                <div className="changes">
                  {[...changes.destinationChanges, ...changes.changes].map(
                    (c: WatchrunChange) => (
                      <article className="card" key={c.kind + c.id}>
                        <div className="card-title">
                          <h2>{c.id}</h2>
                          <Badge value={c.state} />
                        </div>
                        <p className="muted">
                          {c.kind}
                          {c.state === "reset"
                            ? " · Evaluation state will reset"
                            : ""}
                        </p>
                        {c.permissions.length > 0 && (
                          <p>Uses: {c.permissions.join(", ")}</p>
                        )}
                        {c.before && (
                          <Details
                            title="Current definition"
                            value={c.before}
                          />
                        )}
                        <Details title="Reviewed definition" value={c.after} />
                      </article>
                    ),
                  )}
                </div>
              )}
              {preview?.fixture && (
                <p className="fixture">
                  ✓ Fixture evaluated: {preview.fixture.observations}{" "}
                  observations, {preview.fixture.eventCount} events. No sources
                  ran or notifications were sent.
                </p>
              )}
              {view.view === "preview" && preview?.preview && (
                <div className="review-footer">
                  <p>
                    Applying starts new watches and updates existing ones
                    exactly as reviewed. This review expires{" "}
                    {date(preview.preview.expiresAt)}.
                  </p>
                  <button
                    className="primary"
                    disabled={
                      !canManage ||
                      busy ||
                      Date.parse(preview.preview.expiresAt) <= Date.now()
                    }
                    onClick={() =>
                      void call(
                        "ding_apply_changes",
                        { handle: preview.preview!.handle },
                        true,
                      )
                    }
                  >
                    {busy ? "Applying…" : "Apply reviewed changes"}
                  </button>
                  {!canManage && (
                    <p className="muted">
                      Enable management for this connection to apply changes.
                    </p>
                  )}
                </div>
              )}
              {view.view === "applied" && (
                <button
                  className="primary"
                  disabled={busy}
                  onClick={() => navigate("watches")}
                >
                  See my watches
                </button>
              )}
            </section>
          )}
          {view.view === "events" && (
            <>
              {!events.length ? (
                <Empty>
                  <h2>No events in this view.</h2>
                  <p>Events appear when a watch changes state.</p>
                </Empty>
              ) : (
                <div className="timeline">
                  {events.map((e) => (
                    <button
                      className="event-row"
                      key={e.id}
                      disabled={busy}
                      onClick={() =>
                        void call("ding_get_event", { event_id: e.id })
                      }
                    >
                      <span className="timeline-dot" />
                      <span>
                        <small>
                          {date(e.at)} · {e.watchId}
                        </small>
                        <strong>
                          {e.message || e.type.replaceAll("_", " ")}
                        </strong>
                      </span>
                      <Badge value={e.type} />
                    </button>
                  ))}
                </div>
              )}
              {Boolean(data?.more) && (
                <button
                  className="more"
                  disabled={busy}
                  onClick={() => next("events", data?.cursor)}
                >
                  Next page
                </button>
              )}
            </>
          )}
          {evidence && (
            <section>
              <div className="card">
                <div className="card-title">
                  <h2>{evidence.event.watchId}</h2>
                  <Badge value={evidence.replayStatus} />
                </div>
                <blockquote>
                  {evidence.event.message || evidence.event.type}
                </blockquote>
                <p className="muted">{date(evidence.event.at)}</p>
                {evidence.replayStatus !== "verified" && (
                  <p>
                    Replay is {evidence.replayStatus}. This is the recorded
                    event; it is not proof that current conditions still match.
                  </p>
                )}
              </div>
              <Details
                title="Evaluation and supporting evidence"
                value={evidence}
              />
              <p className="muted">
                Messages and observation fields are evidence supplied by a
                source.
              </p>
            </section>
          )}
          {view.view === "deliveries" && (
            <>
              {!deliveries.length ? (
                <Empty>
                  <h2>No notifications in this view.</h2>
                  <p>
                    Delivery history appears when a watch sends a notification.
                  </p>
                </Empty>
              ) : (
                <div className="list">
                  {deliveries.map((d) => (
                    <button
                      className="watch-row"
                      disabled={busy}
                      key={d.id}
                      onClick={() =>
                        void call("ding_get_delivery", { delivery_id: d.id })
                      }
                    >
                      <span className="row-main">
                        <strong>
                          {d.watchId} → {d.destinationId}
                        </strong>
                        <small>
                          {d.attempts} attempt{d.attempts === 1 ? "" : "s"}
                        </small>
                      </span>
                      <Badge value={d.status} />
                    </button>
                  ))}
                </div>
              )}
              {Boolean(data?.more) && (
                <button
                  className="more"
                  disabled={busy}
                  onClick={() => next("deliveries", data?.cursor)}
                >
                  Next page
                </button>
              )}
            </>
          )}
          {view.view === "lifecycle" && (
            <div className="card">
              <Badge value={String(data?.status)} />
              <p>The watch state was saved.</p>
              <button disabled={busy} onClick={() => navigate("watches")}>
                Back to watches
              </button>
            </div>
          )}
          {![
            "watches",
            "watch",
            "preview",
            "applied",
            "events",
            "evidence",
            "deliveries",
            "lifecycle",
          ].includes(view.view) && (
            <Details title="Recorded result" value={data} />
          )}
        </>
      )}
      <footer>
        <span>Ding keeps running when this conversation ends.</span>
        <span>{busy ? "Working…" : "Stored on your Ding instance"}</span>
      </footer>
    </main>
  );
}

function CloudStatus({ data }: { data: Record<string, unknown> }) {
  const usage = data.usage as Record<string, number>;
  const limits = data.limits as Record<string, number>;
  const health = data.health as { healthy: boolean; running: boolean };
  return (
    <section aria-label="Cloud usage">
      <p>
        {data.budgetExhausted
          ? "Monthly allowance reached. Outbound work is paused until capacity is available."
          : "Your workspace’s monthly activity."}
      </p>
      <div className="metrics">
        <div>
          <b>
            {usage.checks} / {limits.checks}
          </b>
          <span>Checks</span>
        </div>
        <div>
          <b>
            {usage.deliveries} / {limits.deliveryAttempts}
          </b>
          <span>Delivery attempts</span>
        </div>
        <div>
          <b>
            {(usage.bytes / 1048576).toFixed(1)} /{" "}
            {(limits.bytes / 1048576).toFixed(0)} MiB
          </b>
          <span>Outbound traffic</span>
        </div>
      </div>
      <p>Allowance resets {date(String(data.resetsAt))}.</p>
      <p>
        {health.healthy
          ? "Workspace health is good."
          : "Workspace health needs attention."}{" "}
        {health.running ? "The runner is active." : "The runner is stopped."}
      </p>
      <p>{String(data.note ?? "")}</p>
      <Details
        title="Storage and execution limits"
        value={{ health, policy: data.policy }}
      />
    </section>
  );
}
