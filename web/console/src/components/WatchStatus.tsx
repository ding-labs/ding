import { CircleCheck, CirclePause, CircleAlert, Radio } from "lucide-react";
import { Link } from "react-router-dom";
import type {
  StoreConsoleWatch,
  StoreWatchSummary,
  WatchDefinition,
} from "../api/contracts";
import { useExecution } from "../app/execution";
import { ErrorBox, Time, labels } from "./common";
import "./WatchOverview.css";

export function WatchContext({ definition }: { definition: WatchDefinition }) {
  const { mode } = useExecution();
  const source = definition.spec.source;
  const parts = source.every?.match(/\d+(?:\.\d+)?(?:ms|s|m|h)/g);
  const units: Record<string, string> = {
    ms: "millisecond",
    s: "second",
    m: "minute",
    h: "hour",
  };
  const every =
    parts?.join("") === source.every && parts?.length
      ? parts
          .filter((part) => parseFloat(part) > 0)
          .map((part) => {
            const value = parseFloat(part);
            const unit = units[part.replace(/[\d.]/g, "")];
            return `${value} ${unit}${value === 1 ? "" : "s"}`;
          })
          .join(" ")
      : source.every;
  return (
    <span className="watch-context">
      {source.type === "push"
        ? "On incoming inputs"
        : source.type === "logs"
          ? "Live log summaries"
          : every
            ? `Every ${every}`
            : "On source input"}
      <span aria-hidden="true"> · </span>
      {mode === "cloud"
        ? "Ding Cloud"
        : mode === "shared"
          ? "Shared server"
          : "This computer"}
    </span>
  );
}

export function WatchStatus({
  detail,
  summary,
  loading,
  error,
  retry,
}: {
  detail: StoreWatchSummary;
  summary?: StoreConsoleWatch;
  loading: boolean;
  error: unknown;
  retry: () => void;
}) {
  const w = detail.watch;
  const { source, condition, policy, destinations } = w.plan.definition.spec;
  const id = w.plan.definition.metadata.id;
  const current = error ? undefined : summary;
  const hasInput = Boolean(w.lastInputAt && !w.lastInputAt.startsWith("0001"));
  const sourceError = Boolean(w.lastError || current?.unhealthy);
  const missing = current?.missing || [];
  const logReason =
    source.type === "logs"
      ? detail.entities.find((entity) => entity.waitingReason)?.waitingReason ||
        String(
          detail.logObservation?.fields?.reason ||
            detail.logObservation?.detail ||
            "",
        )
      : "";
  const waitingForLogs = source.type === "logs" && logReason !== "ready";
  const eventOnly = ["changed", "new-event"].includes(condition.operator || "");
  const incidents =
    current && policy.trigger === "transition" && !eventOnly
      ? `${current.open} open incident${current.open === 1 ? "" : "s"}`
      : undefined;
  const stopped = w.status !== "running";
  const needsAttention =
    sourceError ||
    missing.length > 0 ||
    Boolean(error) ||
    Boolean(current?.open);
  const clear =
    !stopped &&
    !needsAttention &&
    Boolean(current?.entities) &&
    !waitingForLogs &&
    incidents !== undefined;
  const Icon = stopped
    ? CirclePause
    : needsAttention
      ? CircleAlert
      : clear
        ? CircleCheck
        : Radio;
  const title = stopped
    ? w.status === "paused"
      ? "Watch paused"
      : "Watch deleted"
    : !current
      ? loading
        ? "Loading current condition…"
        : "Condition unavailable"
      : current.open && incidents
        ? incidents
        : missing.length
          ? "Credentials need attention"
          : sourceError
            ? "Source needs attention"
            : waitingForLogs
              ? "Waiting for log evidence"
              : !current.entities
                ? "Waiting for first input"
                : condition.operator === "changed"
                  ? "Watching for value changes"
                  : condition.operator === "new-event"
                    ? "Watching for new events"
                    : incidents
                      ? "No open incidents"
                      : `Monitoring ${current.entities} ${current.entities === 1 ? "entity" : "entities"}`;
  const onlyEntity =
    current?.entities === 1 &&
    !detail.entitiesMore &&
    detail.entities.length === 1
      ? detail.entities[0]
      : undefined;

  return (
    <div className="watch-status-stack">
      <section
        className={`panel watch-status-hero ${stopped ? "is-stopped" : needsAttention ? "needs-attention" : clear ? "is-clear" : ""}`}
        aria-labelledby="watch-condition-title"
      >
        <p className="eyebrow">Current condition</p>
        <h2 id="watch-condition-title">
          <Icon size={24} aria-hidden="true" />
          {title}
        </h2>
        {current && (
          <p className="muted watch-condition-context">
            {current.entities} monitored{" "}
            {current.entities === 1 ? "entity" : "entities"}
            {incidents &&
            (stopped || sourceError || waitingForLogs || missing.length > 0)
              ? ` · ${incidents}`
              : ""}
            {onlyEntity &&
            !eventOnly &&
            !stopped &&
            !sourceError &&
            !waitingForLogs &&
            !missing.length &&
            !onlyEntity.open
              ? ` · ${onlyEntity.matches} consecutive ${onlyEntity.matches === 1 ? "match" : "matches"} at the current checkpoint`
              : ""}
          </p>
        )}
        <dl className="watch-check-times">
          <div>
            <dt>Latest input</dt>
            <dd>
              <Time value={w.lastInputAt} />
            </dd>
          </div>
          <div>
            <dt>
              {source.type === "push"
                ? "Input schedule"
                : source.type === "logs"
                  ? "Evaluation schedule"
                  : "Next scheduled check"}
            </dt>
            <dd>
              {stopped ? (
                "Checks stopped"
              ) : source.type === "push" ? (
                "When your source sends an input"
              ) : source.type === "logs" ? (
                "Once per second while collection is active"
              ) : (
                <Time value={w.nextAt} />
              )}
            </dd>
          </div>
        </dl>
        <ErrorBox error={error} retry={retry} />
      </section>
      <div className="watch-status-cards">
        <section className="panel" aria-labelledby="watch-source-title">
          <p className="eyebrow" id="watch-source-title">
            Source
          </p>
          <h3 className="watch-status-value">
            {source.type === "logs"
              ? "Log collection status"
              : sourceError
                ? "Acquisition error"
                : missing.length
                  ? "Missing credentials"
                  : hasInput
                    ? "Input received"
                    : "No input yet"}
          </h3>
          <p
            className={sourceError || missing.length ? "danger-text" : "muted"}
          >
            {w.lastError ||
              (missing.length
                ? `Required credentials: ${missing.join(", ")}`
                : source.type === "logs"
                  ? logReason.replaceAll("_", " ") || "Waiting for collector"
                  : current?.unhealthy
                    ? "Source status is unknown; an incident can remain open."
                    : current
                      ? "No acquisition error reported"
                      : "Source health totals unavailable")}
          </p>
          {hasInput && (
            <details className="watch-disclosure">
              <summary>Latest input details</summary>
              <p>
                Received <Time value={w.lastInputAt} />.
              </p>
              {onlyEntity && (
                <p>
                  Current entity checkpoint: input #{onlyEntity.lastSequence}.
                </p>
              )}
              <Link to={`/watches/${encodeURIComponent(id)}?tab=Entities`}>
                View entity state
              </Link>
            </details>
          )}
        </section>
        <section className="panel" aria-labelledby="watch-notifications-title">
          <p className="eyebrow" id="watch-notifications-title">
            Notifications
          </p>
          <h3 className="watch-status-value">
            {current ? (
              <>
                <span className={current.failed ? "danger-text" : ""}>
                  {current.failed + current.pending}
                </span>{" "}
                <span className="watch-status-unit">failed or pending</span>
              </>
            ) : (
              "Delivery status unavailable"
            )}
          </h3>
          {current && (current.failed > 0 || current.pending > 0) && (
            <p className="muted">
              {current.failed} failed · {current.pending} queued or sending
            </p>
          )}
          <p className="muted">
            {destinations.length ? (
              <>
                Destination{destinations.length === 1 ? "" : "s"}:{" "}
                {destinations.map((d) => d.ref).join(", ")}
              </>
            ) : (
              "No notification destinations configured"
            )}
          </p>
          <details className="watch-disclosure">
            <summary>What gets sent?</summary>
            {destinations.map((destination) => (
              <p key={destination.ref}>
                <Link to="/system?tab=Destinations">{destination.ref}</Link>:{" "}
                {destination.events
                  .map((event) => labels[event] || event)
                  .join(", ")}
                .
              </p>
            ))}
            {!destinations.length && (
              <p>
                Add a destination in the watch definition to receive
                notifications.
              </p>
            )}
            <Link to={`/watches/${encodeURIComponent(id)}?tab=Deliveries`}>
              View deliveries
            </Link>
          </details>
        </section>
      </div>
    </div>
  );
}
