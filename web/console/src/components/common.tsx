import { createContext, useContext, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import {
  ArrowDownToLine,
  ArrowLeft,
  ArrowRight,
  Check,
  Copy,
  RefreshCw,
} from "lucide-react";
import { api, APIError, download } from "../api/client";
import type { WatchDefinition } from "../api/contracts";
export const TimeContext = createContext("local");
export function Time({ value }: { value?: string }) {
  const mode = useContext(TimeContext);
  if (!value || value.startsWith("0001"))
    return <span className="muted">Not yet</span>;
  const d = new Date(value);
  const seconds = (d.getTime() - Date.now()) / 1000;
  const relative =
    Math.abs(seconds) < 60
      ? `${Math.round(Math.abs(seconds))}s`
      : Math.abs(seconds) < 3600
        ? `${Math.round(Math.abs(seconds) / 60)}m`
        : `${Math.round(Math.abs(seconds) / 3600)}h`;
  return (
    <time dateTime={value} title={`${d.toLocaleString()} · ${d.toISOString()}`}>
      {mode === "utc"
        ? d
            .toISOString()
            .replace("T", " ")
            .replace(/\.\d+Z$/, " UTC")
        : mode === "relative"
          ? `${relative} ${seconds > 0 ? "from now" : "ago"}`
          : d.toLocaleString(undefined, {
              month: "short",
              day: "numeric",
              hour: "numeric",
              minute: "2-digit",
              second: "2-digit",
            })}
    </time>
  );
}
export function useRead<T>(
  path: string,
  poll: number | false = false,
  enabled = true,
) {
  return useQuery({
    queryKey: [path],
    queryFn: ({ signal }) => api<T>(path, { signal }),
    refetchInterval: (q) =>
      q.state.error
        ? Math.min((q.state.fetchFailureCount + 1) * 5000, 30000)
        : poll,
    refetchIntervalInBackground: false,
    enabled,
  });
}
export function Heading({
  eyebrow = "Ding Console",
  title,
  description,
  children,
}: {
  eyebrow?: string;
  title: string;
  description?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        {description && <p className="subtitle">{description}</p>}
      </div>
      <div className="actions">{children}</div>
    </div>
  );
}
export const labels: Record<string, string> = {
  firing: "Condition fired",
  recovered: "Incident recovered",
  source_error: "Source unavailable",
  source_recovered: "Source recovered",
  state_reset: "Condition state reset",
  paused: "Paused",
  running: "Running",
  deleted: "Deleted",
  resume: "Watch resumed",
  pause: "Watch paused",
  delete: "Watch deleted",
  gap: "Sampling gap",
  pending: "Queued / retry scheduled",
  leased: "Sending",
  delivered: "Delivered",
  permanent: "Permanent failure",
  exhausted: "Attempts exhausted",
  canceled: "Canceled",
  unknown: "Unknown",
  "new-event": "New provider event",
  changed: "Value changed",
  clock_reset: "Clock reset",
};
export function Badge({
  value,
  children,
}: {
  value: string;
  children?: ReactNode;
}) {
  let tone = [
    "firing",
    "permanent",
    "exhausted",
    "source_error",
    "error",
  ].includes(value)
    ? "danger"
    : ["paused", "pending", "unknown", "warning", "canceled"].includes(value)
      ? "warn"
      : ["running", "delivered", "recovered", "verified", "ok"].includes(value)
        ? "good"
        : "neutral";
  return (
    <span className={`badge ${tone}`}>
      <span className="badge-dot" />
      {children || labels[value] || value.replaceAll("_", " ")}
    </span>
  );
}
export function ErrorBox({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  if (!error) return null;
  const gap = error instanceof APIError && error.status === 410;
  return (
    <div className="notice error" role="alert">
      <strong>
        {gap ? "Retained history has a gap." : "This view could not be loaded."}
      </strong>
      <p>{error instanceof Error ? error.message : String(error)}</p>
      {retry && (
        <button className="button" onClick={retry}>
          <RefreshCw size={14} />
          {gap ? "Load available history" : "Try again"}
        </button>
      )}
    </div>
  );
}
export function Loading() {
  return (
    <div className="loading" role="status">
      <span className="pulse" /> Reading the daemon…
    </div>
  );
}
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <span className="empty-mark">d.</span>
      <h2>{title}</h2>
      <div className="muted">{children}</div>
    </div>
  );
}
export function Raw({
  value,
  name = "ding-data.json",
  title = "Original JSON",
}: {
  value: unknown;
  name?: string;
  title?: string;
}) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const text = open ? JSON.stringify(value, null, 2) : "";
  return (
    <details className="raw" onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary>{title}</summary>
      {open && (
        <>
          <div className="actions">
            <button
              className="button small"
              onClick={async () => {
                await navigator.clipboard.writeText(text);
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              }}
            >
              {copied ? <Check size={14} /> : <Copy size={14} />} Copy
            </button>
            <button
              className="button small"
              onClick={() => download(name, text)}
            >
              <ArrowDownToLine size={14} /> Download
            </button>
          </div>
          <pre>
            {text.slice(0, 16000)}
            {text.length > 16000
              ? "\n… Preview limited to 16,000 characters. Download for the complete record."
              : ""}
          </pre>
        </>
      )}
    </details>
  );
}
export function Filter({
  name,
  label,
  options,
}: {
  name: string;
  label: string;
  options: string[];
}) {
  const [p, set] = useSearchParams();
  return (
    <label className="filter">
      <span>{label}</span>
      <select
        value={p.get(name) || ""}
        onChange={(e) => {
          const n = new URLSearchParams(p);
          n.delete("cursor");
          e.target.value ? n.set(name, e.target.value) : n.delete(name);
          set(n);
        }}
      >
        <option value="">All {label.toLowerCase()}</option>
        {options.map((o) => (
          <option key={o} value={o}>
            {labels[o] || o}
          </option>
        ))}
      </select>
    </label>
  );
}
export function Pager({
  more,
  cursor,
  total,
}: {
  more: boolean;
  cursor: string;
  total?: number;
}) {
  const [p, set] = useSearchParams();
  return (
    <div className="pager">
      <span className="muted">
        {total === undefined
          ? "Bounded page"
          : `${total.toLocaleString()} matching records`}
      </span>
      <div className="actions">
        {p.has("cursor") && (
          <button
            className="button"
            onClick={() => {
              const n = new URLSearchParams(p);
              n.delete("cursor");
              set(n);
            }}
          >
            <ArrowLeft size={14} /> First page
          </button>
        )}
        {more && (
          <button
            className="button"
            onClick={() => {
              const n = new URLSearchParams(p);
              n.set("cursor", cursor);
              set(n);
            }}
          >
            Next page <ArrowRight size={14} />
          </button>
        )}
      </div>
    </div>
  );
}
export function Back({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Link className="back" to={to}>
      <ArrowLeft size={14} />
      {children}
    </Link>
  );
}
export function Rule({ definition: d }: { definition: WatchDefinition }) {
  const s = d.spec;
  const c = s.condition;
  return (
    <div className="rule-summary">
      <p>
        {s.source.type === "push"
          ? "Receive pushed JSON"
          : s.source.type === "command"
            ? "Run a command on the daemon host"
            : `Check ${s.source.url || s.source.urlRef?.env || "an HTTP endpoint"}`}
        {s.source.every ? ` every ${s.source.every}` : ""}.
      </p>
      <div className="rule-expression">
        <code>
          {c.numeric ||
            (c.missingFor
              ? `No observation for ${c.missingFor}`
              : c.operator === "new-event"
                ? `New ${c.field} · deduplicate for ${c.dedupFor}`
                : `${c.field} ${c.operator} ${c.operator === "changed" ? "" : JSON.stringify(c.value)}`)}
        </code>
      </div>
      <p className="muted">
        Trigger: {s.policy.trigger}. {s.policy.consecutive} consecutive match
        {s.policy.consecutive === 1 ? "" : "es"}; recover after{" "}
        {s.policy.recoverAfter} nonmatching inputs. Unknown inputs:{" "}
        {s.policy.onUnknown}.
      </p>
    </div>
  );
}
export function Tabs({
  items,
  active,
  onChange,
}: {
  items: string[];
  active: string;
  onChange: (s: string) => void;
}) {
  return (
    <nav className="tabs" aria-label="Detail sections">
      {items.map((t) => (
        <button
          className={active === t ? "active" : ""}
          key={t}
          onClick={() => onChange(t)}
          aria-current={active === t ? "page" : undefined}
        >
          {t}
        </button>
      ))}
    </nav>
  );
}
