import { useState } from "react";
import { api, unknownOutcome } from "../api/client";
import { ErrorBox, useRead } from "../components/common";

type Connection = { id: string; name: string; provider: string; watch: string };

export function WorkOSSignups() {
  const list = useRead<Connection[]>("/webhooks");
  const [watch, setWatch] = useState("ding-account-signups");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState("");
  const connection = list.data?.find(
    (item) => item.provider === "workos" && item.watch === watch.trim(),
  );

  async function create() {
    setBusy(true);
    setError(undefined);
    setMessage("");
    try {
      const bytes = crypto.getRandomValues(new Uint8Array(32));
      const provisional = Array.from(bytes, (b) =>
        b.toString(16).padStart(2, "0"),
      ).join("");
      await api("/webhooks", {
        method: "POST",
        body: {
          name: "WorkOS signups",
          provider: "workos",
          watch: watch.trim(),
          events: ["user.created"],
          secret: provisional,
        },
      });
      setMessage(
        "Connection created. Add its URL in WorkOS, then save the signing secret below.",
      );
    } catch (e) {
      setError(e);
      setUncertain(unknownOutcome(e));
    } finally {
      await list.refetch();
      setBusy(false);
    }
  }

  async function saveSecret() {
    if (!connection) return;
    setBusy(true);
    setError(undefined);
    setMessage("");
    try {
      await api(`/webhooks/${connection.id}/rotate`, {
        method: "POST",
        body: { secret, overlapSeconds: 0 },
      });
      setSecret("");
      setMessage(
        "Signing secret saved. Send a test event from WorkOS, then check Ding’s Events and Deliveries.",
      );
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel workos-signups" aria-labelledby="workos-signups-heading">
      <h3 id="workos-signups-heading">WorkOS signup alerts</h3>
      <p>
        Create a push watch with a Slack destination first. This connection
        reports AuthKit registrations, including people who have not opened Ding
        yet.
      </p>
      <ErrorBox error={list.error || error} />
      <label>
        Signup watch ID
        <input
          value={watch}
          disabled={busy}
          onChange={(e) => {
            setWatch(e.target.value);
            setSecret("");
            setMessage("");
          }}
        />
      </label>
      {connection ? (
        <>
          <label>
            WorkOS endpoint URL
            <input
              readOnly
              value={`${window.location.origin}/v1/hooks/workos/${connection.id}`}
            />
          </label>
          <p>
            In WorkOS, create a webhook for <strong>user.created</strong> at
            this URL. Copy its signing secret into the field below. Only event
            IDs and times are included in alerts.
          </p>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void saveSecret();
            }}
          >
            <label>
              WorkOS signing secret
              <input
                type="password"
                autoComplete="off"
                minLength={16}
                required
                value={secret}
                disabled={busy}
                onChange={(e) => setSecret(e.target.value)}
              />
            </label>
            <button className="button" disabled={busy || secret.length < 16}>
              Save WorkOS signing secret
            </button>
          </form>
        </>
      ) : (
        <button
          className="button"
          disabled={busy || !watch.trim() || !list.data || uncertain}
          onClick={() => void create()}
        >
          Create signup connection
        </button>
      )}
      {uncertain && !connection && (
        <p role="status">
          The connection outcome is uncertain. Refresh this page and check the
          connection before creating another.
        </p>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
