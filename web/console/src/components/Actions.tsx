import { useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import * as Dialog from "@radix-ui/react-dialog";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  MoreHorizontal,
  Pause,
  Play,
  RotateCcw,
  Trash2,
  X,
} from "lucide-react";
import { watchActions } from "../app/watchLifecycle";
import { useCapability } from "../app/execution";
import { useConnection } from "../api/connection";
import { api, APIError, unknownOutcome } from "../api/client";
import type { StoreWatchRecord, StoreLifecycleGuidance, StoreIntent } from "../api/contracts";
import { ErrorBox } from "./common";
function Confirm({
  title,
  description,
  open,
  onClose,
  children,
  busy,
}: {
  title: string;
  description: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  busy: boolean;
}) {
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(v) => {
        if (!v && !busy) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content
          className="dialog-content"
          onEscapeKeyDown={(e) => {
            if (busy) e.preventDefault();
          }}
        >
          <Dialog.Title>{title}</Dialog.Title>
          <Dialog.Description>{description}</Dialog.Description>
          {children}
          <Dialog.Close asChild>
            <button
              className="dialog-close icon-button"
              aria-label="Close confirmation"
              disabled={busy}
            >
              <X size={18} />
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
export function WatchActions({ watch, lifecycle }: { watch: StoreWatchRecord; lifecycle?: StoreLifecycleGuidance | null }) {
  const allowed = useCapability("manage");
  const [action, setAction] = useState("");
  const [expected, setExpected] = useState("");
  const [expectedGeneration, setExpectedGeneration] = useState(0);
  const [typed, setTyped] = useState("");
  const [cancelPending, setCancelPending] = useState(false);
  const client = useQueryClient();
  const connected = useConnection();
  const id = watch.plan.definition.metadata.id;
  const navigate = useNavigate();
  const finish = async () => {
    setAction("");
    if (action === "purge") {
      navigate("/watches", { replace: true });
      const path = `/watches/${encodeURIComponent(id)}`;
      client.removeQueries({
        predicate: (query) => {
          const key = query.queryKey[0];
          return (
            typeof key === "string" &&
            (key === path || key.startsWith(path + "?"))
          );
        },
      });
    }
    await client.invalidateQueries();
  };
  const change = useMutation({
    mutationFn: () =>
      api(`/watches/${encodeURIComponent(id)}/lifecycle`, {
        method: "POST",
        body: {
          action,
          expected,
          expectedGeneration,
          cancelPending: action === "delete" && cancelPending,
        },
      }),
    onSuccess: finish,
  });
  const start = (kind: string) => {
    setAction(kind);
    setExpected(watch.plan.revision);
    setExpectedGeneration(watch.generation);
    setTyped("");
    setCancelPending(false);
    change.reset();
  };
  if (!allowed) return null;
  const actions = watchActions(watch.status, lifecycle);
  const toggle = actions.find(a => a === "pause" || a === "resume");
  const destructive = action === "delete" || action === "purge";
  const title =
    action === "purge"
      ? `Permanently delete ${id}?`
      : action === "delete"
        ? `Delete ${id}?`
        : action === "pause"
          ? `Pause ${id}?`
          : `Resume ${id}?`;
  return (
    <>
      {actions.includes("purge") && (
        <button
          className="button danger"
          disabled={!connected}
          onClick={() => start("purge")}
        >
          <Trash2 size={15} /> Permanently delete…
        </button>
      )}
      {toggle && (
        <button className="button" disabled={!connected} onClick={() => start(toggle)}>
          {toggle === "resume" ? <Play size={15} /> : <Pause size={15} />}{" "}
          {toggle === "resume" ? "Resume watch" : "Pause watch"}
        </button>
      )}
      {actions.includes("delete") && (
        <Menu.Root>
          <Menu.Trigger asChild>
            <button
              className="icon-button"
              aria-label="More watch actions"
              disabled={!connected}
            >
              <MoreHorizontal size={18} />
            </button>
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="dropdown" align="end" sideOffset={6}>
              <Menu.Item
                className="danger-text"
                onSelect={() => start("delete")}
              >
                <Trash2 size={14} /> Delete watch…
              </Menu.Item>
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      )}
      <Confirm
        title={title}
        description={
          action === "purge"
            ? "Permanently removes this watch from the list, deletes its recorded history, and cancels queued deliveries. This cannot be undone, and this watch ID cannot be reused."
            : action === "delete"
              ? "Delete stops this watch permanently. Recorded history remains available, and this watch ID cannot be reused."
              : action === "pause"
                ? "Pause stops new acquisition and timers. Committed deliveries continue. Incident state is retained."
                : "Resume enables acquisition and timers. The runtime records the gap; interrupted consecutive-match progress is not treated as continuous."
        }
        open={!!action}
        onClose={() => setAction("")}
        busy={change.isPending}
      >
        <p className="subtle">
          Revision {expected.slice(0, 12)} · {window.location.host}
        </p>
        {action === "delete" && (
          <>
            <fieldset className="radio-options">
              <legend>Existing delivery queue</legend>
              <label>
                <input
                  type="radio"
                  name="cancel"
                  checked={!cancelPending}
                  onChange={() => setCancelPending(false)}
                />{" "}
                Let committed deliveries continue
              </label>
              <label>
                <input
                  type="radio"
                  name="cancel"
                  checked={cancelPending}
                  onChange={() => setCancelPending(true)}
                />{" "}
                Cancel pending deliveries
              </label>
            </fieldset>
            <p className="notice">
              An in-flight notification may already have been received.
              Cancellation cannot retract it.
            </p>
          </>
        )}
        {action === "purge" && (
          <p className="notice">
            An in-flight notification may already have been received. Permanent
            deletion cannot retract it.
          </p>
        )}
        {destructive && (
          <label className="stack-label">
            Type <code>{id}</code> to confirm
            <input
              aria-label="Watch ID to delete"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
            />
          </label>
        )}
        <ErrorBox error={change.error} />
        {change.isError && unknownOutcome(change.error) && (
          <div className="notice">
            <strong>The outcome is unknown.</strong>
            <p>
              The operation may have committed. This confirmation will not
              submit it again.
            </p>
            <button
              className="button"
              onClick={async () => {
                try {
                  await api(`/watches/${encodeURIComponent(id)}`);
                  await client.invalidateQueries();
                  setAction("");
                } catch (error) {
                  if (
                    action === "purge" &&
                    error instanceof APIError &&
                    error.status === 404
                  ) {
                    await finish();
                  }
                  // Otherwise keep the uncertain action visible until a read succeeds.
                }
              }}
            >
              Inspect current state
            </button>
          </div>
        )}
        {change.isError && (
          <p className="muted">
            Close this confirmation and refresh the watch before trying again if
            its revision changed.
          </p>
        )}
        <div className="dialog-actions">
          <button
            className="button"
            disabled={change.isPending}
            onClick={() => setAction("")}
          >
            {change.isError && unknownOutcome(change.error)
              ? "Close"
              : "Keep current state"}
          </button>
          <button
            className={`button ${destructive ? "danger" : "primary"}`}
            disabled={
              !actions.includes(action) ||
              change.isPending ||
              !connected ||
              (change.isError && unknownOutcome(change.error)) ||
              (destructive && typed !== id)
            }
            onClick={() => change.mutate()}
          >
            {change.isPending
              ? "Committing…"
              : action === "purge"
                ? "Permanently delete watch"
                : action === "delete"
                  ? "Delete watch"
                  : action === "pause"
                    ? "Pause watch"
                    : "Resume watch"}
          </button>
        </div>
      </Confirm>
    </>
  );
}
export function RetryAction({ intent }: { intent: StoreIntent }) {
 const allowed = useCapability("retry");
  const [open, setOpen] = useState(false);
  const client = useQueryClient();
  const connected = useConnection();
  const retry = useMutation({
    mutationFn: () =>
      api(`/deliveries/${intent.id}/retry`, { method: "POST", body: {} }),
    onSuccess: async () => {
      await client.invalidateQueries();
      setOpen(false);
    },
  });
  if (!allowed) return null;
  if (!["permanent", "exhausted", "canceled"].includes(intent.status) && !open)
    return null;
  return (
    <>
      <button
        className="button"
        disabled={!connected}
        onClick={() => {
          retry.reset();
          setOpen(true);
        }}
      >
        <RotateCcw size={15} />
        Retry delivery
      </button>
      <Confirm
        title={`Retry delivery #${intent.id}?`}
        description="This starts a new policy cycle for the original notification. An earlier attempt may have been received, so retrying can produce a duplicate."
        open={open}
        onClose={() => setOpen(false)}
        busy={retry.isPending}
      >
        <dl className="facts">
          <div>
            <dt>Original event</dt>
            <dd>
              <code>{intent.eventId}</code>
            </dd>
          </div>
          <div>
            <dt>Destination</dt>
            <dd>{intent.destinationId}</dd>
          </div>
          <div>
            <dt>Recorded revision</dt>
            <dd>
              <code>{intent.destinationRevision}</code>
            </dd>
          </div>
        </dl>
        <ErrorBox error={retry.error} />
        {retry.isError && unknownOutcome(retry.error) && (
          <div className="notice">
            <strong>The outcome is unknown.</strong>
            <p>
              The retry may already be queued. This confirmation will not submit
              it again.
            </p>
            <button
              className="button"
              onClick={async () => {
                try {
                  await api(`/deliveries/${intent.id}`);
                  await client.invalidateQueries();
                  setOpen(false);
                } catch {
                  /* Retain the uncertain state. */
                }
              }}
            >
              Inspect current state
            </button>
          </div>
        )}
        <div className="dialog-actions">
          <button
            className="button"
            disabled={retry.isPending}
            onClick={() => setOpen(false)}
          >
            Cancel
          </button>
          <button
            className="button primary"
            disabled={
              retry.isPending ||
              !connected ||
              (retry.isError && unknownOutcome(retry.error))
            }
            onClick={() => retry.mutate()}
          >
            {retry.isPending ? "Queueing…" : "Retry original notification"}
          </button>
        </div>
      </Confirm>
    </>
  );
}
