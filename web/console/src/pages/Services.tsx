import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import { useExecution } from "../app/execution";
import { ErrorBox, useRead } from "../components/common";
type Service = {
  id: string;
  name: string;
  scopes: string[];
  watches: string[];
  expiresAt: string;
  token?: string;
  keyId?: string;
};
type Key = { id: string; expiresAt: string; lastUsedAt?: string };
function ServiceKeys({
  service,
  busy,
  changed,
}: {
  service: Service;
  busy: boolean;
  changed: () => void;
}) {
  const [open,setOpen]=useState(false);
  const keys = useRead<Key[]>(`/services/${service.id}/keys`,false,open);
  const [error, setError] = useState<unknown>();
  async function revoke(id: string) {
    try {
      await api(`/services/${service.id}/keys/${id}`, { method: "DELETE" });
      await keys.refetch();
      changed();
    } catch (e) {
      setError(e);
    }
  }
  return (
    <details onToggle={e=>setOpen(e.currentTarget.open)}>
      <summary>Key metadata and individual revocation</summary>
      <ErrorBox error={error || keys.error} />
      <ul>
        {keys.data?.map((k) => (
          <li key={k.id}>
            <code>{k.id.slice(0, 12)}</code> · expires{" "}
            {new Date(k.expiresAt).toLocaleString()} · last accepted use{" "}
            {k.lastUsedAt
              ? new Date(k.lastUsedAt).toLocaleString()
              : "not recorded"}{" "}
            <button
              className="button"
              disabled={busy}
              onClick={() => void revoke(k.id)}
            >
              Revoke key
            </button>
          </li>
        ))}
      </ul>
    </details>
  );
}
function ServiceResources({
  service,
  changed,
}: {
  service: Service;
  changed: () => void;
}) {
  const [watches, setWatches] = useState(service.watches.join(", "));
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      await api(`/services/${service.id}`, {
        method: "PATCH",
        body: {
          name: service.name,
          scopes: service.scopes,
          watches: watches
            .split(",")
            .map((v) => v.trim())
            .filter(Boolean),
        },
      });
      changed();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  if (!service.scopes.includes("ingest")) return null;
  return (
    <details>
      <summary>Edit allowed watches</summary>
      <form onSubmit={save}>
        <label>
          Allowed push watch IDs
          <input
            required
            value={watches}
            onChange={(e) => setWatches(e.target.value)}
          />
        </label>
        <button className="button" disabled={busy}>
          Save allowed watches
        </button>
      </form>
      <ErrorBox error={error} />
    </details>
  );
}
export function Services() {
  const execution = useExecution();
  const info = useRead<{
    push?: { supported: boolean; admissionOpen: boolean };
  }>("/info");
  const ingest =
    execution.mode === "shared" || Boolean(info.data?.push?.supported);
  const list = useRead<Service[]>("/services");
  const [name, setName] = useState("");
  const [watch, setWatch] = useState("");
  const [scope, setScope] = useState("inspect");
  const [token, setToken] = useState("");
  const [days, setDays] = useState(30);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [revision, setRevision] = useState(0);
  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    setToken("");
    try {
      const result = await api<Service>("/services", {
        method: "POST",
        body: {
          name,
          scopes: [scope],
          watches:
            scope === "ingest"
              ? watch
                  .split(",")
                  .map((v) => v.trim())
                  .filter(Boolean)
              : [],
          days,
        },
      });
      setToken(result.token || "");
      await list.refetch();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function revoke(id: string) {
    setBusy(true);
    setToken("");
    try {
      await api(`/services/${id}`, { method: "DELETE" });
      await list.refetch();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function rotate(id: string) {
    setBusy(true);
    setToken("");
    setError(undefined);
    try {
      const result = await api<Service>(`/services/${id}/rotate`, {
        method: "POST",
        body: { days, overlapSeconds: 3600 },
      });
      setToken(result.token || "");
      setRevision((r) => r + 1);
      await list.refetch();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel">
      <h2>Service credentials</h2>
      <p>
        Workspace-owned automation keeps running when its creator leaves. Give
        each producer access to its selected watches. Rotation preserves retry
        history and allows the previous key for one hour.
      </p>
      <ErrorBox error={error || list.error} />
      {info.data?.push && !info.data.push.admissionOpen && (
        <p role="status">
          New cloud input is temporarily closed. Existing receipts can still be
          reconciled.
        </p>
      )}
      {token && (
        <div role="status">
          <p>Save this credential privately now. It is shown only once.</p>
          <input
            type="password"
            aria-label="New service credential"
            readOnly
            value={token}
          />
          <button
            className="button"
            onClick={() => void navigator.clipboard.writeText(token)}
          >
            Copy credential
          </button>
          <button className="button" onClick={() => setToken("")}>
            Dismiss
          </button>
        </div>
      )}
      <p className="muted">
        If an issuance response is lost, inspect the credential list and key
        metadata. Rotate to obtain a replacement, then revoke the uncertain key.
        Secrets cannot be retrieved again.
      </p>
      <label>
        New key expiry in days
        <input
          type="number"
          min={1}
          max={90}
          required
          value={days}
          onChange={(e) => setDays(Number(e.target.value))}
        />
      </label>
      <ul>
        {list.data?.map((s) => (
          <li key={s.id}>
            {s.name} · {s.scopes.join(", ")} · {s.watches.join(", ")} · expires{" "}
            {new Date(s.expiresAt).toLocaleString()}{" "}
            <button
              className="button"
              disabled={busy}
              onClick={() => void rotate(s.id)}
            >
              Rotate key
            </button>{" "}
            <button
              className="button"
              disabled={busy}
              onClick={() => void revoke(s.id)}
            >
              Revoke producer
            </button>
            <ServiceResources service={s} changed={() => void list.refetch()} />
            <ServiceKeys
              key={`${s.id}:${revision}`}
              service={s}
              busy={busy}
              changed={() => void list.refetch()}
            />
          </li>
        ))}
      </ul>
      <form className="onboarding-form" onSubmit={create}>
        <fieldset disabled={busy}>
          <label>
            Name
            <input
              required
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          <label>
            Permission
            <select aria-label="Service permission" value={scope} onChange={(e) => setScope(e.target.value)}>
              <option value="inspect">Inspect this workspace</option>
              <option value="retry">Retry deliveries in this workspace</option>
              {ingest && (
                <option value="ingest">Send input to selected watches</option>
              )}
            </select>
          </label>
          {scope === "ingest" && (
            <label>
              Existing push watch IDs, separated by commas
              <input
                required
                value={watch}
                onChange={(e) => setWatch(e.target.value)}
              />
            </label>
          )}
          <button className="button">Create credential</button>
        </fieldset>
      </form>
    </section>
  );
}
