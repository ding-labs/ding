import { useState, type FormEvent } from "react";
import { RefreshCw } from "lucide-react";
import { api } from "../api/client";
import { ErrorBox, Time, useRead } from "../components/common";
import {
  CredentialDialog,
  CredentialError,
  KeyExpiry,
  watchIDs,
  type IssuedCredential,
} from "./ServiceCredentialDialogs";

export type Service = {
  id: string;
  name: string;
  scopes: string[];
  watches: string[];
  expiresAt: string;
};
type ServiceKey = { id: string; expiresAt: string; lastUsedAt?: string };
export const permissionNames: Record<string, string> = {
  inspect: "Inspect",
  retry: "Retry deliveries",
  ingest: "Send input",
};
type View = "details" | "rotate" | "watches" | "revoke";

export function ServiceCredentialDetails({
  service,
  onClose,
  onIssued,
  refresh,
}: {
  service: Service;
  onClose: () => void;
  onIssued: (credential: IssuedCredential) => void;
  refresh: () => Promise<unknown>;
}) {
  const path = `/services/${encodeURIComponent(service.id)}`;
  const keys = useRead<ServiceKey[]>(`${path}/keys`);
  const [view, setView] = useState<View>("details");
  const [keyToRevoke, setKeyToRevoke] = useState<ServiceKey>();
  const [watches, setWatches] = useState("");
  const [days, setDays] = useState(30);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  function show(next: View, key?: ServiceKey) {
    setError(undefined);
    setKeyToRevoke(key);
    setView(next);
    if (next === "rotate") setDays(30);
    if (next === "watches") setWatches(service.watches.join(", "));
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(undefined);
    if (view === "watches" && !watchIDs(watches).length) {
      setError(new Error("Select at least one existing push watch."));
      return;
    }
    setBusy(true);
    try {
      if (view === "rotate") {
        const result = await api<{ token?: string }>(`${path}/rotate`, {
          method: "POST",
          body: { days, overlapSeconds: 3600 },
        });
        if (!result.token)
          throw new Error("The server did not return the new key.");
        onIssued({ token: result.token, name: service.name, rotated: true });
      } else if (view === "watches") {
        await api(path, {
          method: "PATCH",
          body: {
            name: service.name,
            scopes: service.scopes,
            watches: watchIDs(watches),
          },
        });
        show("details");
      } else if (view === "revoke") {
        await api(
          keyToRevoke
            ? `${path}/keys/${encodeURIComponent(keyToRevoke.id)}`
            : path,
          { method: "DELETE" },
        );
        if (keyToRevoke) {
          await keys.refetch();
          show("details");
        } else onClose();
      }
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
      // Reconcile metadata even when an issuance response was lost.
      if (view === "rotate") void keys.refetch();
      await refresh();
    }
  }
  const title =
    view === "details"
      ? "Credential details"
      : view === "rotate"
        ? "Rotate key"
        : view === "watches"
          ? "Edit allowed watches"
          : keyToRevoke
            ? "Revoke key?"
            : "Revoke credential?";
  const cancel = () => (view === "details" ? onClose() : show("details"));
  return (
    <CredentialDialog
      title={title}
      description={service.name}
      onClose={cancel}
      busy={busy}
    >
      {view === "details" ? (
        <>
          <dl className="service-properties">
            <div>
              <dt>Permission</dt>
              <dd>
                {service.scopes
                  .map((scope) => permissionNames[scope] || scope)
                  .join(", ")}
              </dd>
            </div>
            {service.scopes.includes("ingest") && (
              <div>
                <dt>Watch access</dt>
                <dd>
                  <ul className="service-watch-list">
                    {service.watches.map((watch) => (
                      <li key={watch}>{watch}</li>
                    ))}
                  </ul>
                  <button
                    className="text-button"
                    onClick={() => show("watches")}
                  >
                    Edit watches
                  </button>
                </dd>
              </div>
            )}
            {service.scopes.some((scope) => scope !== "ingest") && (
              <div>
                <dt>Scope</dt>
                <dd>Entire workspace</dd>
              </div>
            )}
            <div>
              <dt>Owner</dt>
              <dd>This workspace</dd>
            </div>
          </dl>
          <div className="service-key-heading">
            <h3>Keys</h3>
            <button className="text-button" onClick={() => show("rotate")}>
              <RefreshCw size={14} />
              Rotate key
            </button>
          </div>
          <ErrorBox error={keys.error} retry={() => void keys.refetch()} />
          {keys.isPending && (
            <p className="muted" role="status">
              Loading key metadata…
            </p>
          )}
          {keys.data?.length === 0 && (
            <p className="service-hint">
              No keys remain. Rotate to create a new key for this credential.
            </p>
          )}
          <ul className="service-key-list">
            {keys.data?.map((key) => (
              <li key={key.id}>
                <div className="service-key-heading">
                  <code title={key.id}>{key.id.slice(0, 12)}</code>
                  <span className="muted">
                    {new Date(key.expiresAt).getTime() <= Date.now()
                      ? "Expired"
                      : "Valid"}
                  </span>
                </div>
                <dl className="service-key-properties">
                  <div>
                    <dt>Expires</dt>
                    <dd>
                      <Time value={key.expiresAt} />
                    </dd>
                  </div>
                  <div>
                    <dt>Last accepted use</dt>
                    <dd>
                      {key.lastUsedAt ? (
                        <Time value={key.lastUsedAt} />
                      ) : (
                        "Not recorded"
                      )}
                    </dd>
                  </div>
                </dl>
                <button
                  className="text-button"
                  aria-label={`Revoke key ${key.id.slice(0, 12)}`}
                  onClick={() => show("revoke", key)}
                >
                  Revoke key
                </button>
              </li>
            ))}
          </ul>
          <details className="service-more">
            <summary>More actions</summary>
            <button className="button" onClick={() => show("revoke")}>
              Revoke credential
            </button>
          </details>
        </>
      ) : (
        <form className="service-form" onSubmit={submit}>
          <fieldset disabled={busy}>
            {view === "rotate" && (
              <>
                <p className="service-hint">
                  The previous key remains valid for up to one hour. Retry
                  history stays intact.
                </p>
                <KeyExpiry days={days} onChange={setDays} />
              </>
            )}
            {view === "watches" && (
              <>
                <label>
                  Allowed push watch IDs
                  <input
                    required
                    autoComplete="off"
                    value={watches}
                    onChange={(e) => setWatches(e.target.value)}
                  />
                </label>
                <small>
                  Separate existing push watch IDs with commas. Removing a watch
                  immediately removes this credential’s access to it.
                </small>
              </>
            )}
            {view === "revoke" && (
              <p>
                {keyToRevoke ? (
                  <>
                    Key <code>{keyToRevoke.id.slice(0, 12)}</code> will stop
                    working immediately. Automations using this key will lose
                    access.
                  </>
                ) : (
                  "All keys for this credential will stop working immediately. Automations using them will lose access."
                )}
              </p>
            )}
            <CredentialError
              error={error}
              action={
                view === "rotate"
                  ? "Could not rotate key."
                  : view === "watches"
                    ? "Could not save watch access."
                    : "Could not revoke access."
              }
              issuance={view === "rotate"}
            />
            <div className="dialog-actions">
              <button type="button" className="button" onClick={cancel}>
                Cancel
              </button>
              <button
                className={`button ${view === "revoke" ? "danger" : "primary"}`}
                type="submit"
              >
                {busy
                  ? "Saving…"
                  : view === "rotate"
                    ? "Rotate key"
                    : view === "watches"
                      ? "Save allowed watches"
                      : keyToRevoke
                        ? "Revoke key"
                        : "Revoke credential"}
              </button>
            </div>
          </fieldset>
        </form>
      )}
    </CredentialDialog>
  );
}
