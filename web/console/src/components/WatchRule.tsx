import { Link } from "react-router-dom";
import type { StoreEntitySummary, WatchDefinition } from "../api/contracts";
import { ConditionExpression } from "./common";

export function WatchRule({
  definition: d,
  entity,
  entityCount,
}: {
  definition: WatchDefinition;
  entity?: StoreEntitySummary;
  entityCount?: number;
}) {
  const { source, condition, policy } = d.spec;
  const eventOnly = ["changed", "new-event"].includes(condition.operator || "");
  const transition = policy.trigger === "transition";
  const unit =
    source.type === "http" || source.type === "command" ? "check" : "input";
  const recovering = transition && entity?.open;
  const target = recovering ? policy.recoverAfter : policy.consecutive;
  const count = recovering ? entity?.recoveries : entity?.matches;
  const progress = entity && !condition.missingFor && !eventOnly;
  return (
    <section className="panel watch-rule" aria-labelledby="watch-rule-title">
      <h2 id="watch-rule-title">What this watch is looking for</h2>
      <p className="watch-rule-lead">
        {eventOnly ? (
          <>
            {condition.operator === "changed"
              ? "Record each change to "
              : "Record each new value of "}
            <code>{condition.field}</code>.
          </>
        ) : condition.missingFor ? (
          <>
            Open an incident after <strong>{condition.missingFor}</strong>{" "}
            without an observation.
          </>
        ) : (
          <>
            {transition ? "Open an incident" : "Send an event"} when{" "}
            <code>
              <ConditionExpression condition={condition} />
            </code>
            <br />
            for{" "}
            <strong>
              {policy.consecutive} consecutive {unit}
              {policy.consecutive === 1 ? "" : "s"}
            </strong>
            .
          </>
        )}
      </p>
      {progress && (
        <div className="watch-rule-progress">
          <progress
            aria-label={
              recovering ? "Recovery progress" : "Consecutive matches"
            }
            max={target}
            value={Math.min(count || 0, target)}
          />
          <span>
            {count} of {target} consecutive{" "}
            {recovering ? "nonmatching" : "matching"} {unit}
            {target === 1 ? "" : "s"}
            {recovering ? " to recover" : ""}
          </span>
          {entity.sourceUnhealthy && (
            <span className="watch-progress-note">
              Source unknown · retained checkpoint
            </span>
          )}
        </div>
      )}
      {entityCount !== undefined && entityCount > 1 && !eventOnly && (
        <p className="muted">
          Progress is tracked separately for each entity.{" "}
          <Link
            to={`/watches/${encodeURIComponent(d.metadata.id)}?tab=Entities`}
          >
            View {entityCount} entities
          </Link>
        </p>
      )}
      <details className="watch-disclosure">
        <summary>
          {transition && !eventOnly
            ? "Recovery and source settings"
            : "Event and source settings"}
        </summary>
        {condition.operator === "new-event" ? (
          <p>Deduplicate values for {condition.dedupFor}.</p>
        ) : (
          !eventOnly && (
            <p>
              {transition ? (
                <>
                  Recover after {policy.recoverAfter} consecutive {unit}
                  {policy.recoverAfter === 1 ? "" : "s"} that do not match.
                </>
              ) : (
                <>Minimum interval between events: {policy.interval}.</>
              )}
            </p>
          )
        )}
        {!eventOnly && (
          <p>If an input is unknown, preserve the current incident state.</p>
        )}
        <p>
          {source.type === "push" ? (
            "Receive pushed JSON."
          ) : source.type === "command" ? (
            "Run a command on the host."
          ) : source.type === "logs" ? (
            "Read host logs."
          ) : (
            <>
              Check{" "}
              <code>
                {source.url || source.urlRef?.env || "an HTTP endpoint"}
              </code>
              .
            </>
          )}
          {source.every && <> Every {source.every}.</>}
        </p>
        <Link
          to={`/watches/${encodeURIComponent(d.metadata.id)}?tab=Definition`}
        >
          View full definition
        </Link>
      </details>
    </section>
  );
}
