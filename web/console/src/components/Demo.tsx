import { useEffect, useRef, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Play, X } from "lucide-react";
import { stream } from "../api/client";
import type { DemoUpdate, WatchEvent } from "../api/contracts";
import { useExecution } from "../app/execution";
import { NotificationSetup } from "./NotificationSetup";

const stages: Record<string, string> = {
  starting: "Starting a temporary local watch…",
  healthy: "The endpoint is healthy (HTTP 200). Ding is checking it.",
  failing:
    "The endpoint now returns HTTP 503. Waiting for three failing checks…",
  recovering: "The endpoint is healthy again. Waiting for two recovery checks…",
  complete:
    "Ding recorded a firing and a recovery. The temporary watch and its data have been removed.",
  failed: "The demonstration could not finish.",
};

export function Demo() {
  const cloud = useExecution().mode === "cloud";
  const [open, setOpen] = useState(false);
  const [desktop, setDesktop] = useState(false);
  const [busy, setBusy] = useState(false);
  const [stage, setStage] = useState("");
  const [events, setEvents] = useState<WatchEvent[]>([]);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), []);
  if (cloud) return null;

  const close = () => {
    request.current?.abort();
    setBusy(false);
    setOpen(false);
  };
  async function run() {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError("");
    setEvents([]);
    setStage("starting");
    let finished = false;
    try {
      await stream<DemoUpdate>(
        "/demo",
        { desktop },
        controller.signal,
        (update) => {
          if (update.stage) setStage(update.stage);
          if (update.event)
            setEvents((previous) => [...previous, update.event!]);
          if (update.error) setError(update.error);
          if (update.stage === "complete" || update.stage === "failed")
            finished = true;
        },
      );
      if (!finished)
        throw new Error(
          "The connection ended before Ding confirmed completion. Run the demo again to verify it.",
        );
    } catch (e) {
      if (!controller.signal.aborted) {
        setStage("failed");
        setError(e instanceof Error ? e.message : "The demo could not finish.");
      }
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(value) => {
        if (!value) close();
      }}
    >
      <button
        className="button"
        onClick={() => {
          setOpen(true);
          setStage("");
          setEvents([]);
          setError("");
        }}
      >
        <Play size={16} />
        Try a one-minute demo
      </button>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content demo-dialog">
          <Dialog.Title>Watch an alert happen</Dialog.Title>
          <Dialog.Description>
            A real local endpoint goes healthy → failing → recovered. Ding
            checks it, records the evidence, and cleans up afterward. Your
            existing watches stay untouched.
          </Dialog.Description>
          <p className="notice">
            This is a temporary demonstration. It does not monitor your service.
          </p>
          <div className="demo-notification-options">
            <label>
              <input
                type="checkbox"
                checked={desktop}
                disabled={busy}
                onChange={(e) => setDesktop(e.target.checked)}
              />{" "}
              Also send desktop notifications
            </label>
            {desktop && !busy ? <NotificationSetup /> : null}
          </div>
          <div className="demo-progress" role="status" aria-live="polite">
            <p>
              {stages[stage] ||
                "Ready when you are. No endpoint or account needed."}
            </p>
          </div>
          {events.length ? (
            <ol className="demo-events">
              {events.map((event) => (
                <li key={event.id}>
                  <strong>
                    {event.type === "firing"
                      ? "Alert fired"
                      : "Recovery recorded"}
                  </strong>
                  <span>
                    HTTP {String(event.fields?.["http.status"] ?? "")} ·{" "}
                    {new Date(event.at).toLocaleTimeString()}
                  </span>
                </li>
              ))}
            </ol>
          ) : null}
          {error ? (
            <p className="notice" role="alert">
              {error}
            </p>
          ) : null}
          <div className="actions">
            <button
              className="button primary"
              disabled={busy}
              onClick={() => void run()}
            >
              {busy ? "Demo running…" : stage ? "Run again" : "Start demo"}
            </button>
            <button className="button" onClick={close}>
              {busy ? "Stop demo" : "Close"}
            </button>
          </div>
          <Dialog.Close asChild>
            <button
              className="dialog-close icon-button"
              aria-label="Close demonstration"
            >
              <X size={18} />
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
