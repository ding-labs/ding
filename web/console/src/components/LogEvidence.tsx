import type { WatchLogSource, WatchObservation } from "../api/contracts";
import { Time } from "./common";

const reasons: Record<string, string> = {
  warming_up: "Building the first complete window",
  insufficient_records: "Waiting for enough output",
  insufficient_coverage: "Waiting for enough recognized severity labels",
  collection_gap: "Collection gap; rebuilding reliable evidence",
  invalid_records: "Invalid or oversized records in this window",
  collector_stalled: "Collector stopped publishing summaries",
  collector_reconnected: "Collector reconnected; rebuilding the window",
  collection_ended: "Collection ended",
  log_reader_ended: "Log reader exited; waiting to reconnect",
  log_reader_failed: "Log reader failed; waiting to reconnect",
  log_file_unavailable: "Log file unavailable; waiting to reconnect",
  ready: "Collecting; window ready for evaluation",
};

export function LogEvidence({
  id,
  config,
  observation,
  status,
}: {
  id: string;
  config: WatchLogSource;
  observation?: WatchObservation | null;
  status: string;
}) {
  const f = observation?.fields || {};
  const stale =
    f.source_status === "active" &&
    observation &&
    Date.now() - Date.parse(observation.acceptedAt) >= 5000;
  const reason = stale
    ? "collector_stalled"
    : String(f.reason || observation?.detail || "");
  const count = (name: string) =>
    typeof f[name] === "number" ? f[name].toLocaleString() : "—";
  const ratio = (name: string) =>
    typeof f[name] === "number" && f.ready === true && !stale
      ? `${(f[name] * 100).toFixed(1)}%`
      : "Waiting";
  const unit = config.framing === "multiline" ? "records" : "lines";
  const watchArg = `'${id.replaceAll("'", "'\\''")}'`;
  return (
    <section className="panel">
      <h2>Live log evidence</h2>
      <p role="status">
        {status !== "running"
          ? `Watch ${status}`
          : reasons[reason] ||
            (reason ? reason.replaceAll("_", " ") : "Waiting for a collector")}
      </p>
      <dl className="facts">
        <div>
          <dt>Detected warnings</dt>
          <dd>
            {ratio("warning_match_ratio")} · {count("warning_count")} /{" "}
            {count("total_count")} {unit}
          </dd>
        </div>
        <div>
          <dt>Detected errors or fatal</dt>
          <dd>
            {ratio("error_match_ratio")} · {count("error_count")} errors +{" "}
            {count("fatal_count")} fatal
          </dd>
        </div>
        <div>
          <dt>Classification coverage</dt>
          <dd>
            {typeof f.classification_coverage === "number"
              ? `${(100 * f.classification_coverage).toFixed(1)}%`
              : "—"}{" "}
            · {count("unclassified_count")} unclassified
          </dd>
        </div>
        <div>
          <dt>Collection quality</dt>
          <dd>
            {count("invalid_count")} invalid · {count("dropped_analysis_count")}{" "}
            known analysis drops
          </dd>
        </div>
        <div>
          <dt>Channels</dt>
          <dd>
            {count("stdout_count")} stdout · {count("stderr_count")} stderr ·{" "}
            {count("other_channel_count")} merged / file / unknown
          </dd>
        </div>
        <div>
          <dt>Window</dt>
          <dd>
            <Time
              value={
                typeof f.window_start === "string" ? f.window_start : undefined
              }
            />{" "}
            →{" "}
            <Time
              value={
                typeof f.window_end === "string" ? f.window_end : undefined
              }
            />
          </dd>
        </div>
        <div>
          <dt>Latest summary</dt>
          <dd>
            <Time value={observation?.acceptedAt} />
          </dd>
        </div>
        <div>
          <dt>Parser / framing</dt>
          <dd>
            {config.format} · {config.textProfile || "common"} ·{" "}
            {config.framing || "line"}
          </dd>
        </div>
        <div>
          <dt>Upstream lag</dt>
          <dd>
            {typeof f.lag_bytes === "number"
              ? `${count("lag_bytes")} bytes`
              : "Unavailable for this input"}
          </dd>
        </div>
        {typeof f.exit_code === "number" && (
          <div>
            <dt>Application exit code</dt>
            <dd>{f.exit_code}</dd>
          </div>
        )}
        {typeof f.session === "string" && (
          <div>
            <dt>Session</dt>
            <dd className="mono">{f.session}</dd>
          </div>
        )}
      </dl>
      <p>
        Ratios use all collected {unit}, including unclassified output. Quiet or
        incomplete windows hold any open incident. Required: {config.minEvents}{" "}
        {unit} over {config.window}, with {100 * (config.minCoverage || 0)}%
        classification coverage.
      </p>
      {config.input.type === "external" ? (
        <>
          <p>
            Capture runs in your terminal until the application or pipe ends:
          </p>
          <pre>{`ding logs exec --watch ${watchArg} -- your-command\nyour-command 2>&1 | ding logs ingest --watch ${watchArg}`}</pre>
        </>
      ) : (
        <p>
          Reader runs on the daemon host:{" "}
          <code>{config.input.path || JSON.stringify(config.input.argv)}</code>.
          Pausing stops this reader.
        </p>
      )}
    </section>
  );
}
