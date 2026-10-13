import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Check, Copy, Eye, EyeOff, KeyRound, X } from "lucide-react";
import { api, unknownOutcome } from "../api/client";
import "./services.css";

export type IssuedCredential = {
  token: string;
  name: string;
  rotated?: boolean;
};

export function CredentialDialog({
  title,
  description,
  children,
  onClose,
  busy = false,
  locked = false,
}: {
  title: string;
  description: string;
  children: ReactNode;
  onClose: () => void;
  busy?: boolean;
  locked?: boolean;
}) {
  const opener = useRef(document.activeElement as HTMLElement | null);
  const content = useRef<HTMLDivElement>(null);
  const previousTitle = useRef(title);
  useLayoutEffect(() => {
    if (previousTitle.current !== title) {
      previousTitle.current = title;
      (
        content.current?.querySelector<HTMLElement>("input, select") ||
        content.current
      )?.focus();
    }
  }, [title]);
  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open && !busy && !locked) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content
          ref={content}
          className="dialog-content service-dialog"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (opener.current?.isConnected) opener.current.focus();
            else document.querySelector<HTMLElement>(".services-add")?.focus();
          }}
          onEscapeKeyDown={(e) => {
            if (busy || locked) e.preventDefault();
          }}
          onInteractOutside={(e) => e.preventDefault()}
        >
          <Dialog.Title>{title}</Dialog.Title>
          <Dialog.Description>{description}</Dialog.Description>
          {children}
          {!locked && (
            <Dialog.Close asChild>
              <button
                type="button"
                className="dialog-close icon-button"
                aria-label="Close dialog"
                disabled={busy}
              >
                <X size={18} />
              </button>
            </Dialog.Close>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function CredentialError({
  error,
  action,
  issuance = false,
}: {
  error: unknown;
  action: string;
  issuance?: boolean;
}) {
  if (!error) return null;
  return (
    <div className="notice error" role="alert">
      <strong>{action}</strong>
      <p>{error instanceof Error ? error.message : String(error)}</p>
      {issuance && unknownOutcome(error) && (
        <p>
          The key may have been created. Close this dialog and check the
          credential details. Rotate to get a replacement, then revoke the
          uncertain key. Secrets cannot be retrieved again.
        </p>
      )}
    </div>
  );
}

export function KeyExpiry({
  days,
  onChange,
}: {
  days: number;
  onChange: (days: number) => void;
}) {
  const [custom, setCustom] = useState(false);
  return (
    <div className="service-field">
      <label>
        Key expires in
        <select
          value={custom ? "custom" : days}
          onChange={(e) => {
            setCustom(e.target.value === "custom");
            if (e.target.value !== "custom") onChange(Number(e.target.value));
          }}
        >
          <option value="7">7 days</option>
          <option value="30">30 days</option>
          <option value="90">90 days</option>
          <option value="custom">Custom duration…</option>
        </select>
      </label>
      {custom && (
        <label>
          Expiry in days
          <input
            type="number"
            min={1}
            max={90}
            step={1}
            required
            value={Number.isNaN(days) ? "" : days}
            onChange={(e) => onChange(e.target.valueAsNumber)}
          />
        </label>
      )}
      <small>You can rotate the key before it expires. Maximum 90 days.</small>
    </div>
  );
}

export const watchIDs = (value: string) => [
  ...new Set(
    value
      .split(",")
      .map((id) => id.trim())
      .filter(Boolean),
  ),
];

export function CreateCredential({
  ingest,
  onClose,
  onIssued,
  refresh,
}: {
  ingest: boolean;
  onClose: () => void;
  onIssued: (credential: IssuedCredential) => void;
  refresh: () => void;
}) {
  const hintId = useId();
  const [name, setName] = useState("");
  const [permission, setPermission] = useState("inspect");
  const [watches, setWatches] = useState("");
  const [days, setDays] = useState(30);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  async function create(e: FormEvent) {
    e.preventDefault();
    setError(undefined);
    if (
      !name.trim() ||
      (permission === "ingest" && !watchIDs(watches).length)
    ) {
      setError(
        new Error(
          "Enter a credential name and at least one watch ID when sending input.",
        ),
      );
      return;
    }
    setBusy(true);
    try {
      const result = await api<{ token?: string }>("/services", {
        method: "POST",
        body: {
          name: name.trim(),
          scopes: [permission],
          watches: permission === "ingest" ? watchIDs(watches) : [],
          days,
        },
      });
      if (!result.token)
        throw new Error("The server did not return the new key.");
      onIssued({ token: result.token, name: name.trim() });
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
      refresh();
    }
  }
  return (
    <CredentialDialog
      title="Add credential"
      description="Give an automation the access it needs."
      onClose={onClose}
      busy={busy}
    >
      <form className="service-form" onSubmit={create}>
        <fieldset disabled={busy}>
          <div className="service-field">
            <label>
              Credential name
              <input
                aria-describedby={`${hintId}-name`}
                required
                maxLength={100}
                autoComplete="off"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. production-event-producer"
              />
            </label>
            <small id={`${hintId}-name`}>
              Use a name that describes the automation.
            </small>
          </div>
          <label>
            Permission
            <select
              value={permission}
              onChange={(e) => setPermission(e.target.value)}
            >
              <option value="inspect">Inspect this workspace</option>
              <option value="retry">Retry deliveries in this workspace</option>
              {ingest && (
                <option value="ingest">Send input to selected watches</option>
              )}
            </select>
          </label>
          {permission === "ingest" && (
            <div className="service-field">
              <label>
                Allowed push watch IDs
                <input
                  aria-describedby={`${hintId}-watches`}
                  required
                  autoComplete="off"
                  value={watches}
                  onChange={(e) => setWatches(e.target.value)}
                />
              </label>
              <small id={`${hintId}-watches`}>
                Separate existing push watch IDs with commas. Only these watches
                can receive input.
              </small>
            </div>
          )}
          <KeyExpiry days={days} onChange={setDays} />
          <div className="service-hint">
            <KeyRound size={16} />
            <span>You’ll see the secret once, after creation.</span>
          </div>
          <CredentialError
            error={error}
            action="Could not create credential."
            issuance
          />
          <div className="dialog-actions">
            <button type="button" className="button" onClick={onClose}>
              Cancel
            </button>
            <button className="button primary" type="submit">
              {busy ? "Creating…" : "Create credential"}
            </button>
          </div>
        </fieldset>
      </form>
    </CredentialDialog>
  );
}

export function CredentialSecret({
  credential,
  onClose,
}: {
  credential: IssuedCredential;
  onClose: () => void;
}) {
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  const [visible, setVisible] = useState(false);
  const [error, setError] = useState<unknown>();
  async function copy() {
    setError(undefined);
    try {
      await navigator.clipboard.writeText(credential.token);
      setCopied(true);
    } catch {
      setError(
        new Error(
          "Copying is unavailable. Reveal the key, select it, and copy it manually.",
        ),
      );
    }
  }
  return (
    <CredentialDialog
      title={credential.rotated ? "New key created" : "Credential created"}
      description="Save the secret now. You won’t be able to view it again."
      onClose={onClose}
      locked
    >
      <p className="service-secret-name">{credential.name}</p>
      <div className="service-secret">
        <label>
          New service credential
          <input
            type={visible ? "text" : "password"}
            autoComplete="off"
            spellCheck={false}
            readOnly
            value={credential.token}
            onFocus={(e) => e.target.select()}
          />
        </label>
        <div className="actions">
          <button className="button small" onClick={() => void copy()}>
            {copied ? <Check size={14} /> : <Copy size={14} />}
            {copied ? "Copied" : "Copy key"}
          </button>
          <button
            className="button small"
            onClick={() => setVisible((v) => !v)}
            aria-pressed={visible}
          >
            {visible ? <EyeOff size={14} /> : <Eye size={14} />}
            {visible ? "Hide key" : "Reveal key"}
          </button>
        </div>
        <span className="sr-only" role="status">
          {copied ? "Key copied to clipboard." : ""}
        </span>
      </div>
      <CredentialError error={error} action="Could not copy key." />
      {credential.rotated && (
        <p className="service-hint">
          The previous key remains valid for up to one hour. Update your
          automation before then.
        </p>
      )}
      <label className="service-saved">
        <input
          type="checkbox"
          checked={saved}
          onChange={(e) => setSaved(e.target.checked)}
        />
        I’ve saved this key
      </label>
      <div className="dialog-actions">
        <button className="button primary" disabled={!saved} onClick={onClose}>
          Done
        </button>
      </div>
    </CredentialDialog>
  );
}
