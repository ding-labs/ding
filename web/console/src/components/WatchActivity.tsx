import { ArrowUpRight, FileCheck2, Activity } from "lucide-react";
import { Link } from "react-router-dom";
import type { StoreEventSummary } from "../api/contracts";
import { ErrorBox, Time, labels } from "./common";

export function WatchActivity({
  watchID,
  event,
  transition,
  loading,
  error,
  retry,
}: {
  watchID: string;
  event?: StoreEventSummary;
  transition: boolean;
  loading: boolean;
  error: unknown;
  retry: () => void;
}) {
  const configuration = event?.type === "applied";
  const title = !event
    ? ""
    : configuration
      ? event.message === "created"
        ? "Watch created"
        : event.message === "preserved"
          ? "Definition reapplied"
          : "Definition applied"
      : event.type === "firing" && transition
        ? "Incident opened"
        : labels[event.type] || event.type.replaceAll("_", " ");
  const message =
    configuration && event.message === "preserved"
      ? "Existing incident state preserved"
      : configuration && event.message === "created"
        ? "Initial definition saved"
        : event?.message;
  const Icon = configuration ? FileCheck2 : Activity;
  return (
    <section className="watch-activity" aria-labelledby="watch-activity-title">
      <div className="watch-activity-heading">
        <h2 id="watch-activity-title">
          {configuration
            ? "Latest configuration activity"
            : "Latest recorded event"}
        </h2>
        <Link to={`/events?watch=${encodeURIComponent(watchID)}`}>
          All events <ArrowUpRight size={14} aria-hidden="true" />
        </Link>
      </div>
      <ErrorBox error={error} retry={retry} />
      {!error &&
        (loading ? (
          <p className="muted" role="status">
            Loading activity…
          </p>
        ) : event ? (
          <div className="watch-activity-row">
            <Icon size={19} aria-hidden="true" />
            <div>
              <h3>{title}</h3>
              <p className="muted">{message}</p>
              {event.entity && event.entity !== "[]" && (
                <small>{event.entity}</small>
              )}
            </div>
            <div className="watch-activity-meta">
              <Time value={event.at} />
              <Link to={`/events/${encodeURIComponent(event.id)}`}>
                Inspect event <ArrowUpRight size={14} aria-hidden="true" />
              </Link>
            </div>
          </div>
        ) : (
          <p className="muted">
            No recorded events yet. A watch can receive inputs without producing
            an event.
          </p>
        ))}
    </section>
  );
}
