import { useState } from "react";
import { Download, FileUp } from "lucide-react";
import type { ControlConversion } from "../api/contracts";
import { api, download } from "../api/client";
import { useDraft, setDraft } from "../app/draft";
import { Badge, ErrorBox, Raw } from "../components/common";
export function LegacyImport({ onEdit }: { onEdit: () => void }) {
  const draft = useDraft();
  const [result, setResult] = useState<ControlConversion>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  return (
    <>
      <div className="notice">
        <strong>Convert, then review.</strong>
        <p>
          Legacy import creates candidate push watches and a conversion report.
          It does not start watches, run sources, or import old snapshots and
          baselines.
        </p>
      </div>
      <div className="editor-toolbar">
        <label className="button">
          <FileUp size={14} />
          Import legacy YAML
          <input
            className="sr-only"
            type="file"
            aria-label="Import legacy YAML"
            accept=".yaml,.yml"
            onChange={async (e) => {
              const f = e.target.files?.[0];
              if (!f) return;
              if (f.size > 1 << 20) {
                setError(new Error("Legacy configuration limit is 1 MiB."));
                return;
              }
              setDraft({ legacy: await f.text(), dirty: true });
              setResult(undefined);
            }}
          />
        </label>
        <button
          className="button primary"
          disabled={busy || !draft.legacy}
          onClick={async () => {
            setBusy(true);
            setError(undefined);
            try {
              setResult(
                await api<ControlConversion>("/tools/migrate", {
                  method: "POST",
                  body: { manifest: draft.legacy },
                }),
              );
            } catch (e) {
              setError(e);
            } finally {
              setBusy(false);
            }
          }}
        >
          {busy ? "Converting…" : "Convert legacy configuration"}
        </button>
      </div>
      <textarea
        className="fixture-editor"
        aria-label="Legacy YAML"
        value={draft.legacy}
        onChange={(e) => {
          setDraft({ legacy: e.target.value, dirty: true });
          setResult(undefined);
        }}
        spellCheck={false}
        placeholder="Paste your old ding.yaml configuration…"
      />
      <ErrorBox error={error} />
      {result && (
        <>
          <section className="panel">
            <div className="section-heading">
              <div>
                <h2>
                  {result.report.converted} converted ·{" "}
                  {result.report.unsupported} unsupported
                </h2>
                <p>{result.report.state}</p>
              </div>
              <button
                className="button"
                onClick={() =>
                  download(
                    "ding-migration.zip",
                    Uint8Array.from(atob(result.archive), (c) =>
                      c.charCodeAt(0),
                    ),
                    "application/zip",
                  )
                }
              >
                <Download size={14} />
                Download conversion archive
              </button>
            </div>
            {result.report.warnings?.map((w) => (
              <div className="migration-warning" key={w.code}>
                <strong>{w.code.replaceAll("_", " ")}</strong>
                <p>{w.message}</p>
              </div>
            ))}
            <h3>Required bindings</h3>
            {result.report.bindings?.map((b, i) => (
              <p key={i}>
                <code>{b.environment}</code> for {b.notifier} · {b.from}
              </p>
            ))}
            {!result.report.bindings?.length && (
              <p className="muted">No additional environment bindings.</p>
            )}
          </section>
          <section className="panel">
            <h2>Rule review</h2>
            {result.report.rules?.map((r) => (
              <article className="description" key={r.id}>
                <h3>{r.name}</h3>
                <Badge value={r.status === "converted" ? "ok" : "warning"}>
                  {r.status}
                </Badge>
                {[...(r.issues || []), ...(r.warnings || [])].map((i, n) => (
                  <p key={n}>{i.message}</p>
                ))}
              </article>
            ))}
          </section>
          {result.files.map((f) => (
            <section className="panel" key={f.name}>
              <div className="check-title">
                <h3>{f.name}</h3>
                {/\.ya?ml$/.test(f.name) && (
                  <button
                    className="button"
                    onClick={() => {
                      if (
                        draft.manifest &&
                        draft.dirty &&
                        !confirm(
                          "Replace the Workbench manifest with this converted file?",
                        )
                      )
                        return;
                      setDraft({ manifest: f.content, dirty: true });
                      onEdit();
                    }}
                  >
                    Open in Workbench
                  </button>
                )}
              </div>
              <details className="raw">
                <summary>Preview converted file</summary>
                <pre>{f.content.slice(0, 16000)}</pre>
              </details>
            </section>
          ))}
          <Raw
            value={result.report}
            name="ding-migration-report.json"
            title="Complete conversion report"
          />
        </>
      )}
    </>
  );
}
