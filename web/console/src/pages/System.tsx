import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Download,
  ExternalLink,
  RefreshCw,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import type {
  ControlInfo,
  ControlBackupArtifact,
  StoreDestinationPage,
  WatchrunDoctor,
  WatchrunStatus,
} from "../api/contracts";
import { api, download } from "../api/client";
import { setDraft, useDraft } from "../app/draft";
import {
  Heading,
  Badge,
  Time,
  useRead,
  ErrorBox,
  Loading,
  Raw,
  Tabs,
  Pager,
  Empty,
} from "../components/common";
import packageInfo from "../../package.json";
import parity from "../../../../testdata/console/parity.json";
import { useExecution } from "../app/execution";
import { CloudSystem } from "./CloudSystem";
function bytes(n: number) {
  return `${(n / (1 << 20)).toFixed(1)} MiB`;
}
export function System() {
  const [p, set] = useSearchParams();
  const cloud = useExecution().mode === "cloud";
  const tabs = cloud ? ["Diagnostics", "Destinations", "Cloud"] : ["Diagnostics", "Destinations", "Backup", "Instance", "CLI setup"];
  const selected = p.get("tab") || "Diagnostics";
  const tab = tabs.includes(selected) ? selected : "Diagnostics";
  const info = useRead<ControlInfo>("/info");
  const status = useRead<WatchrunStatus>("/status", 15000);
  return (
    <>
      <Heading
        title={cloud ? "Know your cloud workspace." : "Know your daemon."}
        eyebrow="System"
        description="Runtime checks, destinations, and the tools to operate this instance."
      >
        <span className="subtle">
          Last status <Time value={status.data?.at} />
        </span>
      </Heading>
      <Tabs
        items={tabs}
        active={tab}
        onChange={(t) => set({ tab: t })}
      />
      <ErrorBox error={info.error || status.error} />
      {tab === "Diagnostics" && <Diagnostics />}
      {tab === "Cloud" && <CloudSystem />}
      {tab === "Destinations" && <Destinations />}
      {tab === "Backup" && <Backup info={info.data} />}{" "}
      {tab === "Instance" && info.data && (
        <Instance info={info.data} status={status.data} />
      )}{" "}
      {tab === "CLI setup" && <Reference />}
    </>
  );
}
function Diagnostics() {
  const [limit, setLimit] = useState(100);
  const d = useQuery({
    queryKey: ["/console/doctor"],
    queryFn: ({ signal }) => api<WatchrunDoctor>("/console/doctor", { signal }),
    refetchOnWindowFocus: false,
    staleTime: 30000,
  });
  return (
    <>
      <div className="section-heading">
        <div>
          <h2>Doctor</h2>
          <p className="subtle">
            Full integrity and runtime checks. Last checked{" "}
            {d.dataUpdatedAt
              ? new Date(d.dataUpdatedAt).toLocaleString()
              : "not yet"}
            .
          </p>
        </div>
        <button
          className="button"
          disabled={d.isFetching}
          onClick={() => void d.refetch()}
        >
          <RefreshCw size={14} />
          {d.isFetching ? "Checking…" : "Run diagnostics"}
        </button>
      </div>
      <ErrorBox error={d.error} />
      {d.isPending ? (
        <Loading />
      ) : (
        d.data && (
          <>
            <div className="diagnostic-grid">
              <section className="panel">
                <div className="check-title">
                  <h3>Runtime</h3>
                  <Badge
                    value={
                      d.data.running && !d.data.closing && !d.data.lastError
                        ? "ok"
                        : "warning"
                    }
                  >
                    {d.data.closing
                      ? "Shutting down"
                      : d.data.running
                        ? "Running"
                        : "Not running"}
                  </Badge>
                </div>
                <p>
                  {d.data.acquisitions} active acquisitions · limit{" "}
                  {d.data.acquisitionLimit}
                </p>
                <p>Delivery workers: {d.data.deliveryLimit}</p>
                {d.data.lastError && (
                  <p className="danger-text">{d.data.lastError}</p>
                )}
              </section>
              <section className="panel">
                <div className="check-title">
                  <h3>Storage integrity</h3>
                  <Badge
                    value={d.data.store.integrity === "ok" ? "ok" : "error"}
                  >
                    {d.data.store.integrity}
                  </Badge>
                </div>
                <p>
                  SQLite {d.data.store.sqlite} · schema {d.data.store.schema} ·{" "}
                  {d.data.store.journal} journal
                </p>
                <p>
                  Database {bytes(d.data.store.databaseBytes)} · WAL{" "}
                  {bytes(d.data.store.walBytes)}
                </p>
                {d.data.store.integrity !== "ok" && (
                  <p>
                    Preserve a copy of the state directory and inspect the
                    daemon logs before changing data.
                  </p>
                )}
              </section>
              <section className="panel">
                <h3>Resource limits</h3>
                <dl className="facts">
                  <div>
                    <dt>Active watches</dt>
                    <dd>
                      {d.data.usage.watches} / {d.data.limits.maxWatches}
                    </dd>
                  </div>
                  <div>
                    <dt>Pending deliveries</dt>
                    <dd>
                      {d.data.usage.pending} / {d.data.limits.maxPending}
                    </dd>
                  </div>
                  <div>
                    <dt>Live data</dt>
                    <dd>
                      {bytes(d.data.usage.liveBytes)} /{" "}
                      {bytes(d.data.limits.maxBytes)}
                    </dd>
                  </div>
                  <div>
                    <dt>History retention</dt>
                    <dd>
                      {d.data.limits.retention / 1e9 / 86400} days (referenced
                      history can remain longer)
                    </dd>
                  </div>
                </dl>
              </section>
              <section className="panel">
                <h3>Delivery queue</h3>
                {Object.entries(d.data.deliveries).map(([state, n]) => (
                  <Link
                    className="check-link"
                    key={state}
                    to={`/deliveries?status=${state}`}
                  >
                    <Badge value={state} />
                    <strong>{n}</strong>
                  </Link>
                ))}
                {!Object.keys(d.data.deliveries).length && (
                  <p className="muted">No delivery intents yet.</p>
                )}
              </section>
            </div>
            <section className="panel">
              <h2>Credentials on the daemon host</h2>
              <p className="muted">
                Presence only. Set environment variables for the process that
                starts Ding; values are never returned here.
              </p>
              {d.data.credentials.map((c) => (
                <div className="entity-row" key={c.environment}>
                  <code>{c.environment}</code>
                  <Badge value={c.present ? "ok" : "warning"}>
                    {c.present ? "Present" : "Missing"}
                  </Badge>
                </div>
              ))}
              {!d.data.credentials.length && (
                <p>No environment references are currently required.</p>
              )}
            </section>
            <section className="panel">
              <h2>Source and condition checks</h2>
              <p className="muted">
                Lifecycle, source health, and open incidents are separate. Old
                push input is not automatically an acquisition failure.
              </p>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Watch</th>
                      <th>Lifecycle</th>
                      <th>Source issues</th>
                      <th>Open incidents</th>
                      <th>Latest accepted input</th>
                      <th>Acquisition</th>
                    </tr>
                  </thead>
                  <tbody>
                    {d.data.sources.slice(0, limit).map((s) => (
                      <tr key={s.id}>
                        <td>
                          <Link to={`/watches/${encodeURIComponent(s.id)}`}>
                            {s.id}
                          </Link>
                        </td>
                        <td>
                          <Badge value={s.status} />
                        </td>
                        <td>
                          {s.lastError ||
                            `${s.unhealthyEntities} unhealthy entities`}
                        </td>
                        <td>{s.openIncidents}</td>
                        <td>
                          <Time value={s.lastInputAt} />
                        </td>
                        <td>
                          <Badge value={s.acquisition} />
                          {s.acquisition === "overdue" && (
                            <p className="subtle">Overdue by {s.overdueSeconds}s. The cause of this gap is unknown.</p>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {d.data.sources.length > limit && (
                <button
                  className="button"
                  onClick={() => setLimit((n) => n + 100)}
                >
                  Show 100 more
                </button>
              )}
            </section>
            <Raw
              value={d.data}
              name="ding-doctor.json"
              title="Complete Doctor response"
            />
          </>
        )
      )}
    </>
  );
}
function Destinations() {
  const [p] = useSearchParams();
  const query = useRead<StoreDestinationPage>(
    `/console/destinations?cursor=${encodeURIComponent(p.get("cursor") || "")}`,
  );
  const nav = useNavigate();
  const draft = useDraft();
  return (
    <>
      <div className="section-heading">
        <div>
          <h2>Where signals go</h2>
          <p className="subtle">
            Current destination definitions. Existing intents retain their
            original revision.
          </p>
        </div>
        <Link className="button" to="/workbench">
          Create a destination
        </Link>
      </div>
      <ErrorBox error={query.error} />
      {query.isPending ? (
        <Loading />
      ) : (
        query.data && (
          <>
            {query.data.destinations.map((d) => (
              <section className="panel" key={d.definition.metadata.id}>
                <div className="check-title">
                  <div>
                    <p className="eyebrow">{d.definition.spec.type}</p>
                    <h2>
                      {d.definition.metadata.name || d.definition.metadata.id}
                    </h2>
                  </div>
                  <button
                    className="button"
                    onClick={() => {
                      if (
                        draft.dirty &&
                        !confirm(
                          "Replace the current Workbench manifest? Download it first if you want to keep it.",
                        )
                      )
                        return;
                      setDraft({
                        manifest: JSON.stringify(d.definition, null, 2),
                        dirty: true,
                      });
                      nav("/workbench");
                    }}
                  >
                    Edit in Workbench
                  </button>
                </div>
                <p>
                  <Link
                    to={`/watches?destination=${encodeURIComponent(d.definition.metadata.id)}`}
                  >
                    {d.watches} watches reference this destination
                  </Link>
                </p>
                <p className="subtle">
                  Revision {d.revision.slice(0, 12)} ·{" "}
                  {d.definition.spec.maxAttempts} attempts ·{" "}
                  {d.definition.spec.maxAge} maximum age
                </p>
                {d.definition.spec.urlRef && (
                  <p>
                    URL from <code>{d.definition.spec.urlRef.env}</code>
                  </p>
                )}
                <Raw
                  value={d.definition}
                  title="Destination definition"
                  name={`${d.definition.metadata.id}.json`}
                />
              </section>
            ))}
            {!query.data.destinations.length && (
              <Empty title="No destinations yet">
                <p>
                  A manifest can define a console, desktop, webhook, Slack, or Discord
                  destination alongside its watches.
                </p>
              </Empty>
            )}
            <Pager cursor={query.data.cursor} more={query.data.more} />
          </>
        )
      )}
    </>
  );
}
function Backup({ info }: { info?: ControlInfo }) {
  const [busy, setBusy] = useState(false);
  const [path, setPath] = useState("");
  const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState("");
  const backup = async (host = false) => {
    setBusy(true);
    setError(undefined);
    setMessage("");
    try {
      if (host) {
        await api("/backup", { method: "POST", body: { path } });
        setMessage(
          `Verified backup saved on ${info?.listen || window.location.host} at ${path}.`,
        );
      } else {
        const d = await api<ControlBackupArtifact>("/console/backup", {
          method: "POST",
          body: {},
        });
        const a = document.createElement("a");
        a.href = `/v1/console/backup/${d.id}`;
        a.download = "ding-backup.db";
        a.click();
        setMessage(
          `Verified ${bytes(d.bytes)} snapshot prepared. Browser download started.`,
        );
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="narrow">
      <section className="panel">
        <ShieldCheck size={26} />
        <h2>A verified copy of your state.</h2>
        <p>
          Ding creates a consistent SQLite snapshot and checks its integrity
          before offering it for download. Watches, events, checkpoints, and
          delivery history are included.
        </p>
        <p className="muted">
          Browser downloads support snapshots up to 512 MiB. Temporary artifacts
          are private to this session, expire after five minutes, and are
          removed after collection. Larger backups can be saved on the daemon
          host.
        </p>
        <button
          className="button primary"
          disabled={busy}
          onClick={() => void backup()}
        >
          <Download size={15} />
          {busy ? "Preparing backup…" : "Download verified backup"}
        </button>
      </section>
      <section className="panel">
        <h2>Save on daemon host</h2>
        <p>
          The file is written on{" "}
          <code>{info?.listen || window.location.host}</code>. Use an absolute
          path in an existing writable directory. Ding never overwrites an
          existing file.
        </p>
        <label className="stack-label">
          Absolute output path
          <input
            aria-label="Backup path on daemon host"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder={
              info?.os === "windows"
                ? "C:\\backups\\ding.db"
                : "/path/to/backups/ding.db"
            }
          />
        </label>
        <button
          className="button"
          disabled={busy || !path}
          onClick={() => void backup(true)}
        >
          Save on daemon host
        </button>
      </section>
      <ErrorBox error={error} />
      {message && (
        <p className="notice" role="status">
          {message}
        </p>
      )}
    </div>
  );
}
function Instance({
  info,
  status,
}: {
  info: ControlInfo;
  status?: WatchrunStatus;
}) {
  return (
    <>
      <div className="overview-grid">
        <section className="panel">
          <h2>About this instance</h2>
          <dl className="facts">
            {Object.entries({
              "Ding binary": info.version,
              "Console build":
                packageInfo.version + " · embedded in this binary",
              API: info.apiVersion,
              Schema: info.schema,
              Platform: `${info.os}/${info.arch}`,
              "Instance ID": status?.instance,
              "Listen address": info.listen,
              "Browser origin": info.origin,
              "State directory": info.stateDir,
            }).map(([k, v]) => (
              <div key={k}>
                <dt>{k}</dt>
                <dd>
                  <code>{v}</code>
                </dd>
              </div>
            ))}
          </dl>
          <Raw value={info} name="ding-instance.json" />
        </section>
        <section className="panel">
          <Terminal size={23} />
          <h2>Start from your terminal.</h2>
          <p>
            The daemon serves this console. Starting a stopped process or
            changing its startup flags happens on its host, through your
            terminal or service manager.
          </p>
          <pre className="command-block">
            ding daemon --state-dir PATH{"\n"}ding ui
          </pre>
          <p>
            Configure limits with <code>--max-watches</code>,{" "}
            <code>--max-pending</code>, <code>--max-store-bytes</code>, and{" "}
            <code>--history</code>. Remote browser access requires a trusted
            HTTPS origin set with <code>--ui-origin</code>.
          </p>
          <Link to="/system?tab=CLI+setup&topic=daemon">
            Read daemon command reference <ExternalLink size={13} />
          </Link>
        </section>
      </div>
      <section className="panel">
        <h2>Legacy command guidance</h2>
        <p>
          <code>run</code>, <code>serve</code>, <code>test-rule</code>, and{" "}
          <code>install</code> belong to the legacy runtime. Existing rules and
          snapshots are not automatically loaded.
        </p>
        <Link className="button" to="/workbench?tab=Legacy+import">
          Convert a legacy configuration
        </Link>{" "}
        <a
          href="https://github.com/ding-labs/ding/releases/tag/v0.14.0"
          target="_blank"
          rel="noreferrer"
        >
          Legacy v0.14.0 release
        </a>
      </section>
    </>
  );
}
function Reference() {
  const [p, set] = useSearchParams();
  const [search, searchFor] = useState("");
  const [shell, setShell] = useState("zsh");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const topic = p.get("topic") || "";
  const ref = useRead<{ text: string }>(
    `/reference?topic=${encodeURIComponent(topic)}`,
  );
  const names = Object.keys(parity).filter(
    (k) => !k.startsWith("completion ") && k.includes(search),
  );
  return (
    <>
      <div className="overview-grid">
        <section className="panel">
          <h2>Command reference</h2>
          <input
            aria-label="Search commands"
            placeholder="Search commands…"
            value={search}
            onChange={(e) => searchFor(e.target.value)}
          />
          <div className="command-list">
            <button
              className={!topic ? "active" : ""}
              onClick={() => set({ tab: "CLI setup" })}
            >
              ding
            </button>
            {names.map((n) => (
              <button
                key={n}
                className={topic === n ? "active" : ""}
                onClick={() => set({ tab: "CLI setup", topic: n })}
              >
                ding {n}
              </button>
            ))}
          </div>
        </section>
        <section className="panel">
          <h2>ding {topic}</h2>
          <ErrorBox error={ref.error} />
          {ref.isPending ? (
            <Loading />
          ) : (
            <pre className="command-block">{ref.data?.text}</pre>
          )}
          <p className="muted">Generated from this binary's command tree.</p>
        </section>
      </div>
      <section className="panel">
        <h2>Shell completion</h2>
        <p>
          Download a completion script generated by this Ding binary. Follow the
          instructions in <code>ding completion {shell} --help</code> for your
          shell.
        </p>
        <div className="actions">
          <select
            aria-label="Completion shell"
            value={shell}
            onChange={(e) => setShell(e.target.value)}
          >
            {["bash", "fish", "powershell", "zsh"].map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
          <button
            className="button"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError(undefined);
              try {
                const d = await api<{ text: string }>(
                  `/reference?topic=completion:${shell}`,
                );
                download(`ding.${shell}`, d.text, "text/plain");
              } catch (e) {
                setError(e);
              } finally {
                setBusy(false);
              }
            }}
          >
            <Download size={14} />
            Download completion
          </button>
        </div>
        <ErrorBox error={error} />
      </section>
    </>
  );
}
