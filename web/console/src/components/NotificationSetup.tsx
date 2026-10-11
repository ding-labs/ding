import { useEffect, useRef, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Bell, X } from "lucide-react";
import { api, APIError } from "../api/client";
import type { ControlInfo, NotifySettings } from "../api/contracts";
import { useRead } from "./common";

export function NotificationSetup({
  onAccepted,
  onTest,
  onUseConsole,
  disabled,
  label = "Set up notifications",
}: {
  onAccepted?: () => void;
  onTest?: () => void;
  onUseConsole?: () => void;
  disabled?: boolean;
  label?: string;
}) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [settings, setSettings] = useState<NotifySettings>();
  const [error, setError] = useState<unknown>();
  const [notice, setNotice] = useState("");
  const request = useRef<AbortController | null>(null);
  const info = useRead<ControlInfo>("/info");
  const mac = (settings?.platform || info.data?.os) === "darwin";
  useEffect(() => () => request.current?.abort(), []);
  useEffect(() => {
    if (!open) return;
    const focus = () => {
      void refresh();
    };
    window.addEventListener("focus", focus);
    return () => window.removeEventListener("focus", focus);
  }, [open]);

  async function refresh() {
    try {
      setSettings(await api<NotifySettings>("/desktop/status"));
    } catch (e) {
      setError(e);
    }
  }
  async function test() {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setAccepted(false);
    setError(undefined);
    setNotice("");
    onTest?.();
    try {
      await api("/desktop/test", {
        method: "POST",
        body: {},
        signal: controller.signal,
      });
      setAccepted(true);
      onAccepted?.();
    } catch (e) {
      if (!controller.signal.aborted) setError(e);
    } finally {
      if (!controller.signal.aborted) {
        setBusy(false);
        void refresh();
      }
    }
  }
  const close = () => {
    request.current?.abort();
    setBusy(false);
    setOpen(false);
  };
  const denied =
    settings?.authorization === "denied" ||
    (error instanceof APIError &&
      error.code === "notification_permission_denied");
  const pending =
    settings?.authorization === "not_determined" ||
    (error instanceof APIError &&
      error.code === "notification_permission_pending");
  const failed =
    error instanceof APIError && error.code === "notification_helper_failed";

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(value) => {
        if (!value) close();
      }}
    >
      <button
        className="button"
        disabled={disabled}
        onClick={() => {
          setOpen(true);
          void refresh();
        }}
      >
        <Bell size={16} />
        {label}
      </button>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content notification-setup">
          <Dialog.Title>Let Ding reach you</Dialog.Title>
          <Dialog.Description>
            Set up desktop alerts on the computer running Ding, then check that
            you can see a test.
          </Dialog.Description>
          <ol className="setup-steps">
            <li>
              <strong>Request permission.</strong> Choose “Send test
              notification” below. If your operating system asks, allow
              notifications for <strong>Ding</strong>. Check Notification Center
              if the prompt is easy to miss.
            </li>
            <li>
              <strong>Check notification settings.</strong>{" "}
              {mac
                ? "Open System Settings → Notifications → Ding. Turn on Allow Notifications and choose banners or alerts."
                : "Open your operating system’s notification settings and enable alerts for Ding. A graphical desktop session is required."}
            </li>
            <li>
              <strong>Send another test.</strong> Check Focus and notification
              delivery settings if permission is enabled but no banner appears.
            </li>
          </ol>
          {mac ? (
            <div className="notice">
              <strong>Ding missing from the list?</strong> Send the test first
              so macOS can register its permission request, then reopen
              Notifications. If the helper fails, changing permissions will not
              repair it; update Ding through Homebrew.
            </div>
          ) : null}
          <div role="status" aria-live="polite">
            {busy ? (
              <p>
                Waiting for your desktop. Respond to Ding’s permission prompt if
                it appears.
              </p>
            ) : null}
            {denied ? (
              <p>
                Notifications are currently turned off for Ding. Enable them in
                Settings, then retry.
              </p>
            ) : null}
            {pending && !busy ? (
              <p>macOS has not recorded a permission choice yet.</p>
            ) : null}
            {settings?.authorization === "authorized" ? (
              <p>
                macOS permission is enabled.
                {settings.alerts === "disabled"
                  ? " Alert banners are turned off."
                  : ""}
              </p>
            ) : null}
            {accepted ? (
              <p>
                The OS accepted the test. Confirm you can actually see it before
                relying on desktop alerts.
              </p>
            ) : null}
            {notice ? <p>{notice}</p> : null}
          </div>
          {error ? (
            <div className="notice" role="alert">
              {failed
                ? "Ding’s notification helper failed. This is an application error, not a confirmed permission denial. Update Ding and retry."
                : error instanceof Error
                  ? error.message
                  : "Notification setup could not finish."}
            </div>
          ) : null}
          <div className="actions">
            <button
              className="button primary"
              disabled={busy}
              onClick={() => void test()}
            >
              {busy
                ? "Waiting for permission…"
                : accepted
                  ? "Send another test"
                  : "Send test notification"}
            </button>
            {mac ? (
              <button
                className="button"
                onClick={async () => {
                  try {
                    await api("/desktop/settings", {
                      method: "POST",
                      body: {},
                    });
                    setNotice(
                      "Settings opened on the Mac running Ding. Return here to send another test.",
                    );
                  } catch (e) {
                    setError(e);
                  }
                }}
              >
                Open notification settings
              </button>
            ) : null}
            <button className="button" onClick={() => void refresh()}>
              Check again
            </button>
          </div>
          {accepted ? (
            <button className="button" onClick={close}>
              Done
            </button>
          ) : null}
          {onUseConsole ? (
            <button
              className="button"
              onClick={() => {
                close();
                onUseConsole();
              }}
            >
              Use Console only for now
            </button>
          ) : null}
          <Dialog.Close asChild>
            <button
              className="dialog-close icon-button"
              aria-label="Close notification setup"
            >
              <X size={18} />
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
