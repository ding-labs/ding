import { useState } from "react";
import { api } from "../api/client";
import type { ControlLogPreview, WatchLogSource } from "../api/contracts";
import { ErrorBox } from "./common";

export function LogPreview({ config }: { config: WatchLogSource }) {
  const [sample, setSample] = useState(
    "WARN retrying\nINFO connected\n0 errors",
  );
  const [response, setResponse] = useState<{
    key: string;
    data: ControlLogPreview;
  }>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const key = JSON.stringify({ config, sample });
  const result = response?.key === key ? response.data : undefined;
  return (
    <section className="panel">
      <h2>Try a log sample</h2>
      <p>
        Use a small sample without secrets. It is sent to your local daemon for
        classification and is not retained. Only the first 100 record labels are
        shown.
      </p>
      <label>
        Sample output
        <textarea
          aria-label="Sample output"
          rows={5}
          maxLength={32000}
          value={sample}
          onChange={(e) => setSample(e.target.value)}
        />
      </label>
      <button
        className="button"
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          setError(undefined);
          try {
            setResponse({
              key,
              data: await api<ControlLogPreview>("/tools/log-preview", {
                method: "POST",
                body: { config, sample },
              }),
            });
          } catch (e) {
            setError(e);
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? "Classifying…" : "Preview classification"}
      </button>
      <ErrorBox error={error} />
      {result && (
        <div role="status">
          <p>
            {result.total} records · {result.warnings} detected warnings ·{" "}
            {result.errors} detected errors or fatal ·{" "}
            {result.total - result.classified} unclassified · {result.invalid}{" "}
            invalid
          </p>
          <p>
            Classification coverage:{" "}
            {result.total
              ? ((100 * result.classified) / result.total).toFixed(1) + "%"
              : "No records"}
            .
          </p>
          <ol>
            {result.levels.map((level, i) => (
              <li key={i}>
                Record {i + 1}: {level}
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}
