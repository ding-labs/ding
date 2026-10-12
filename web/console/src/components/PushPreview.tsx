import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import { ErrorBox, Raw } from "./common";

export function PushPreview({ watch }: { watch: string }) {
  const [body, setBody] = useState("{}");
  const [result, setResult] = useState<unknown>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  async function preview(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    setResult(undefined);
    try {
      const definition = await api<{ manifest: string }>(
        `/watches/${encodeURIComponent(watch)}/export`,
      );
      setResult(
        await api("/tools/project", {
          method: "POST",
          body: { manifest: definition.manifest, watch, fixture: body },
        }),
      );
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <details>
      <summary>Preview JSON projection</summary>
      <p>
        Test field selection and conditions against empty condition state. This
        preview stores no observation and sends no notification. A live request
        may behave differently because of retained condition state.
      </p>
      <form className="onboarding-form" onSubmit={preview}><fieldset disabled={busy}>
        <label>
          Source JSON
          <textarea
            aria-label="Source JSON"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={8}
          />
        </label>
        <button className="button" disabled={busy}>
          Preview JSON
        </button>
      </fieldset></form>
      <ErrorBox error={error} />
      {result !== undefined && (
        <Raw
          value={result}
          title="Projected observations and predicted events"
          name="ding-push-preview.json"
        />
      )}
    </details>
  );
}
