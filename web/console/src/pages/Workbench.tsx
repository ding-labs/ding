import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  CheckCheck,
  Download,
  FileUp,
  Globe,
  Play,
  Radio,
  SquareTerminal,
  X,
} from "lucide-react";
import { api, download, APIError } from "../api/client";
import type {
  ControlCompileResult,
  ControlVerification,
  ReplayReport,
  WatchrunApplyResult,
  ControlInfo,
} from "../api/contracts";
import {
  Heading,
  ErrorBox,
  Loading,
  Badge,
  Raw,
  Time,
  Tabs,
  useRead,
} from "../components/common";
import { LegacyImport } from "./LegacyImport";
import { DefinitionDiff } from "../components/DefinitionDiff";
import { useDraft, setDraft } from "../app/draft";
const base = `apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: console}\nspec: {type: console}\n---\napiVersion: ding.ing/v1alpha1\nkind: Watch\n`;
const examples: Record<string, string> = {
  HTTP:
    base +
    `metadata: {id: api-health, name: API health}\nspec:\n  source:\n    type: http\n    url: https://example.com/health\n    every: 30s\n    timeout: 5s\n  condition: {field: http.status, operator: gte, value: 500}\n  policy: {trigger: transition, consecutive: 3, recoverAfter: 2}\n  destinations: [{ref: console, events: [firing, recovered, source_error, source_recovered]}]\n`,
  Push:
    base +
    `metadata: {id: pushed-status, name: Pushed status}\nspec:\n  source: {type: push}\n  condition: {field: status, operator: gte, value: 500}\n  policy: {trigger: transition, consecutive: 3, recoverAfter: 2}\n  destinations: [{ref: console, events: [firing, recovered]}]\n`,
  Command:
    base +
    `metadata: {id: local-check, name: Local command}\nspec:\n  source:\n    type: command\n    argv: [echo, '{"status":503}']\n    every: 30s\n    timeout: 5s\n  condition: {field: status, operator: gte, value: 500}\n  policy: {trigger: transition, consecutive: 3, recoverAfter: 2}\n  destinations: [{ref: console, events: [firing, recovered]}]\n`,
};
function sampleFixture(field: string) {
  return [503, 502, 500, 200, 200]
    .map((v, i) =>
      JSON.stringify({
        sequence: i + 1,
        inputId: `sample-${i + 1}`,
        acceptedAt: `2026-01-01T00:00:${String(i * 5).padStart(2, "0")}Z`,
        health: "ok",
        fields: { [field]: v },
      }),
    )
    .join("\n");
}
async function readFile(file: File, max: number) {
  if (file.size > max)
    throw new Error(`File exceeds ${max / (1 << 20)} MiB limit.`);
  return file.text();
}
export function Workbench() {
  const draft = useDraft();
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") || "Definition";
  const setTab = (tab: string) => setParams({ tab });
  const [compiled, setCompiled] = useState<ControlCompileResult>();
  const [compiledText, setCompiledText] = useState("");
  const [compiling, setCompiling] = useState(false);
  const [compileError, setCompileError] = useState<unknown>();
  const [validationRun, validate] = useState(0);
  const [selected, select] = useState("");
  const [review, setReview] = useState<WatchrunApplyResult>();
  const [reviewText, setReviewText] = useState("");
  const [error, setError] = useState<unknown>();
  const [result, setResult] = useState<ReplayReport>();
  const [verification, setVerification] = useState<ControlVerification>();
  const [busy, setBusy] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const navigate = useNavigate();
  const client = useQueryClient();
  const info = useRead<ControlInfo>("/info");
  useEffect(() => {
    setReview(undefined);
    setResult(undefined);
    setCompiled(undefined);
    setCompileError(undefined);
    if (!draft.manifest.trim()) {
      setCompiling(false);
      return;
    }
    setCompiling(true);
    const abort = new AbortController();
    const timer = setTimeout(() => {
      api<ControlCompileResult>("/tools/compile", {
        method: "POST",
        body: { manifest: draft.manifest },
        signal: abort.signal,
      })
        .then((d) => {
          setCompiled(d);
          setCompiledText(draft.manifest);
          select((old) =>
            d.bundle.watches?.some((w) => w.definition.metadata.id === old)
              ? old
              : d.bundle.watches?.[0]?.definition.metadata.id || "",
          );
        })
        .catch((e) => {
          if (!abort.signal.aborted) setCompileError(e);
        })
        .finally(() => {
          if (!abort.signal.aborted) setCompiling(false);
        });
    }, 450);
    return () => {
      clearTimeout(timer);
      abort.abort();
    };
  }, [draft.manifest, validationRun]);
  useEffect(() => () => controller.current?.abort(), []);
  const valid =
    compiled?.valid && compiledText === draft.manifest && !compiling;
  const pick = (kind: string) => {
    if (
      draft.dirty &&
      draft.manifest &&
      !confirm(
        "Replace the current draft with this example? Download your draft first if you want to keep it.",
      )
    )
      return;
    setDraft({
      manifest: examples[kind],
      fixture: sampleFixture(kind === "HTTP" ? "http.status" : "status"),
      dirty: true,
    });
    setError(undefined);
    setTab("Definition");
  };
  const run = async (kind: "test" | "replay") => {
    setError(undefined);
    setBusy(true);
    controller.current = new AbortController();
    try {
      if (kind === "test") {
        const d = await api<ReplayReport>("/tools/test", {
          method: "POST",
          body: {
            manifest: draft.manifest,
            fixture: draft.fixture,
            watch: selected,
          },
          signal: controller.current.signal,
        });
        setResult(d);
      } else {
        let evidence;
        try {
          evidence = JSON.parse(draft.evidence);
        } catch {
          setVerification({
            status: "malformed",
            message: "Evidence must be a JSON object.",
          });
          return;
        }
        setVerification(
          await api<ControlVerification>("/tools/replay", {
            method: "POST",
            body: { evidence },
            signal: controller.current.signal,
          }),
        );
      }
    } catch (e) {
      if (!controller.current?.signal.aborted) setError(e);
    } finally {
      setBusy(false);
    }
  };
  const preview = useMutation({
    mutationFn: (manifest: string) =>
      api<WatchrunApplyResult>("/apply", {
        method: "POST",
        body: { manifest, dryRun: true },
      }),
    onSuccess: (d, manifest) => {
      setReview(d);
      setReviewText(manifest);
      setError(undefined);
    },
    onError: setError,
  });
  const commit = useMutation({
    mutationFn: () =>
      api<WatchrunApplyResult>("/apply", {
        method: "POST",
        body: { manifest: reviewText, dryRun: false, review: review?.review },
      }),
    onSuccess: (d) => {
      setDraft({ dirty: false });
      setReview(undefined);
      void client.invalidateQueries();
      navigate(
        d.changes.length
          ? `/watches/${encodeURIComponent(d.changes[0].id)}`
          : "/system?tab=Destinations",
      );
    },
    onError: setError,
  });
  const upload = (key: "manifest" | "fixture" | "evidence", max: number) => (
    <label className="button">
      <FileUp size={14} />
      Import {key}
      <input
        className="sr-only"
        type="file"
        aria-label={`Import ${key}`}
        accept={key === "manifest" ? ".yaml,.yml,.json" : ".json,.jsonl"}
        onChange={async (e) => {
          const f = e.target.files?.[0];
          if (!f) return;
          try {
            const text = await readFile(f, max);
            setDraft({ [key]: text, dirty: true });
            setError(undefined);
          } catch (err) {
            setError(err);
          }
          e.target.value = "";
        }}
      />
    </label>
  );
  if (review && reviewText === draft.manifest)
    return (
      <>
        <Heading
          eyebrow="Workbench / Review changes"
          title="Understand the change. Then apply it."
          description={
            <>
              Receiving daemon{" "}
              <code>{info.data?.listen || window.location.host}</code>.
              Revisions are checked again when you apply.
            </>
          }
        >
          <button
            className="button"
            disabled={commit.isPending}
            onClick={() => {
              setReview(undefined);
              setError(undefined);
            }}
          >
            Back to editor
          </button>
          <button
            className="button primary"
            disabled={commit.isPending}
            onClick={() => commit.mutate()}
          >
            {commit.isPending ? "Applying…" : "Apply reviewed changes"}
            <ArrowRight size={15} />
          </button>
        </Heading>
        <ErrorBox error={error} />
        {error instanceof APIError && error.status === 409 && (
          <div className="notice">
            <p>
              The draft is preserved. Compare with the latest definitions and
              review again before applying.
            </p>
            <button
              className="button"
              disabled={preview.isPending}
              onClick={() => preview.mutate(draft.manifest)}
            >
              Compare with current
            </button>
          </div>
        )}
        <div className="review-summary">
          <div>
            <b>{review.changes.length}</b>
            <span>Watches</span>
          </div>
          <div>
            <b>{review.destinationChanges.length}</b>
            <span>Destinations</span>
          </div>
          <div>
            <b>{review.changes.filter((c) => c.state === "reset").length}</b>
            <span>Condition state resets</span>
          </div>
        </div>
        {review.credentials.some((c) => !c.present) && (
          <div className="notice">
            Missing on the daemon host:{" "}
            {review.credentials
              .filter((c) => !c.present)
              .map((c) => c.environment)
              .join(", ")}
            . Set these environment variables for the daemon to use them. Values
            never enter this browser.
          </div>
        )}
        {[...review.changes, ...review.destinationChanges].map((c) => (
          <section className="panel review-change" key={`${c.kind}/${c.id}`}>
            <div className="section-heading">
              <div>
                <p className="eyebrow">{c.kind}</p>
                <h2>{c.id}</h2>
              </div>
              <Badge value={c.state === "reset" ? "warning" : "neutral"}>
                {c.state}
              </Badge>
            </div>
            <p>
              {c.kind === "Destination"
                ? "New deliveries use this definition; existing delivery intents keep their recorded destination revision."
                : c.state === "reset"
                  ? "Source, condition, grouping, policy or limits changed. The condition checkpoint will reset."
                  : c.state === "created"
                    ? "A new watch starts running after this transaction commits."
                    : "Existing condition state is preserved."}
            </p>
            <p className="subtle">
              {c.previous ? c.previous.slice(0, 12) : "New resource"} →{" "}
              {c.revision.slice(0, 12)}
            </p>
            {c.permissions?.length > 0 && (
              <div className="permission-list">
                <strong>Permissions on the daemon host</strong>
                {c.permissions.map((p) => (
                  <code key={p}>{p}</code>
                ))}
              </div>
            )}
            <DefinitionDiff before={c.before} after={c.after} />
            <div className="definition-diff">
              <div>
                <h3>Current definition</h3>
                <pre>{c.before?.slice(0, 24000) || "Does not exist"}</pre>
              </div>
              <div>
                <h3>Proposed definition</h3>
                <pre>{c.after?.slice(0, 24000)}</pre>
              </div>
            </div>
            {(c.before?.length || 0) > 24000 ||
            (c.after?.length || 0) > 24000 ? (
              <p className="notice">
                Preview shortened. Download the complete review below to inspect
                all content.
              </p>
            ) : null}
          </section>
        ))}
        <Raw
          value={review}
          title="Complete review and revision preconditions"
          name="ding-apply-review.json"
        />
      </>
    );
  return (
    <>
      <Heading
        title="Build a watch you can explain."
        eyebrow="Workbench"
        description="Compile the definition, test the rule, then review exactly what will change."
      >
        <button
          className="button"
          disabled={!draft.manifest}
          onClick={() =>
            download("ding-draft.yaml", draft.manifest, "text/yaml")
          }
        >
          <Download size={15} />
          Download draft
        </button>
        <button
          className="button primary"
          disabled={!valid || preview.isPending}
          onClick={() => preview.mutate(draft.manifest)}
        >
          {preview.isPending ? "Reviewing…" : "Review changes"}
          <ArrowRight size={15} />
        </button>
      </Heading>
      {!draft.manifest && (
        <section className="template-grid">
          <button onClick={() => pick("HTTP")}>
            <Globe size={23} />
            <h2>Inspect an endpoint</h2>
            <p>Poll a URL. Detect unhealthy responses and recovery.</p>
            <span>
              HTTP watch <ArrowRight size={14} />
            </span>
          </button>
          <button onClick={() => pick("Push")}>
            <Radio size={23} />
            <h2>Receive a signal</h2>
            <p>Send JSON from your app, provider, or agent.</p>
            <span>
              Push watch <ArrowRight size={14} />
            </span>
          </button>
          <button onClick={() => pick("Command")}>
            <SquareTerminal size={23} />
            <h2>Watch your machine</h2>
            <p>Run a local command and evaluate its JSON output.</p>
            <span>
              Command watch <ArrowRight size={14} />
            </span>
          </button>
        </section>
      )}
      <Tabs
        items={["Definition", "Simulation", "Verify evidence", "Legacy import"]}
        active={tab}
        onChange={setTab}
      />
      <ErrorBox error={error || compileError} />
      {tab === "Legacy import" && (
        <LegacyImport onEdit={() => setTab("Definition")} />
      )}
      {tab === "Definition" && (
        <>
          <div className="editor-toolbar">
            <div className="actions">
              {upload("manifest", 1 << 20)}
              <select
                aria-label="Choose example"
                value=""
                onChange={(e) => e.target.value && pick(e.target.value)}
              >
                <option value="">Choose example</option>
                {Object.keys(examples).map((k) => (
                  <option key={k}>{k}</option>
                ))}
              </select>
            </div>
            <span className="subtle">
              Draft stays in memory · {draft.manifest.length.toLocaleString()}{" "}
              characters
            </span>
          </div>
          <div className="workbench-grid">
            <section className="editor-panel">
              <div className="editor-label">
                <label htmlFor="manifest">Manifest · YAML or JSON</label>
                <button
                  className="text-button"
                  disabled={!draft.manifest || compiling}
                  onClick={() => validate((n) => n + 1)}
                >
                  Validate / explain
                </button>
              </div>
              <textarea
                ref={editor}
                id="manifest"
                aria-label="Manifest"
                spellCheck={false}
                value={draft.manifest}
                placeholder="Choose an example or import a definition…"
                onChange={(e) =>
                  setDraft({ manifest: e.target.value, dirty: true })
                }
              />
            </section>
            <section className="panel preview-panel">
              <p className="eyebrow">Compiled by Ding</p>
              <h2>The plan, in plain language</h2>
              {compiling ? (
                <Loading />
              ) : compiled ? (
                <>
                  {compiled.valid ? (
                    <>
                      <Badge value="verified">
                        <CheckCheck size={14} /> Valid definition
                      </Badge>
                      {compiled.descriptions.map((d) => (
                        <article className="description" key={d.id}>
                          <h3>{d.id}</h3>
                          <p>{d.source}</p>
                          <div className="rule-expression">{d.condition}</div>
                          <p>{d.policy}</p>
                          <div className="permission-list">
                            {compiled.bundle.watches
                              .find((w) => w.definition.metadata.id === d.id)
                              ?.permissions.map((p) => (
                                <code key={p}>{p}</code>
                              ))}
                          </div>
                        </article>
                      ))}
                      {compiled.bundle.destinations.map((d) => (
                        <div
                          className="description"
                          key={d.definition.metadata.id}
                        >
                          <h3>Destination: {d.definition.metadata.id}</h3>
                          <p>
                            {d.definition.spec.type} ·{" "}
                            {d.definition.spec.maxAttempts} attempts ·{" "}
                            {d.definition.spec.maxAge} maximum age
                          </p>
                          <Raw
                            value={d.definition}
                            title="Destination definition"
                          />
                        </div>
                      ))}
                      {compiled.credentials.map((c) => (
                        <p key={c.environment}>
                          <Badge value={c.present ? "ok" : "warning"}>
                            {c.environment} ·{" "}
                            {c.present ? "present" : "absent on daemon"}
                          </Badge>
                        </p>
                      ))}
                      <Raw
                        value={compiled.bundle}
                        title="Compiled definitions, limits and permissions"
                      />
                    </>
                  ) : (
                    compiled.diagnostics.map((d, i) => (
                      <div className="diagnostic" role="alert" key={i}>
                        <Badge value="error">{d.code}</Badge>
                        <p>{d.message}</p>
                        {d.line && (
                          <button
                            className="text-button"
                            onClick={() => {
                              const pos = draft.manifest
                                .split("\n")
                                .slice(0, d.line! - 1)
                                .reduce((n, l) => n + l.length + 1, 0);
                              editor.current?.focus();
                              editor.current?.setSelectionRange(pos, pos);
                            }}
                          >
                            Go to line {d.line}
                          </button>
                        )}
                      </div>
                    ))
                  )}
                </>
              ) : (
                <p className="muted">
                  The Go compiler validates every schema feature. This preview
                  does not execute the source.
                </p>
              )}
            </section>
          </div>
        </>
      )}
      {tab === "Simulation" && (
        <>
          <div className="notice">
            <strong>Simulation</strong>
            <p>
              Evaluate recorded observations using their accepted timestamps.
              Sources do not execute and notifications are not sent.
            </p>
          </div>
          <div className="editor-toolbar">
            <div className="actions">
              {upload("fixture", 8 << 20)}
              <label>
                Watch{" "}
                <select
                  aria-label="Watch to simulate"
                  value={selected}
                  onChange={(e) => select(e.target.value)}
                >
                  {compiled?.bundle.watches?.map((w) => (
                    <option key={w.definition.metadata.id}>
                      {w.definition.metadata.id}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div className="actions">
              {busy ? (
                <button
                  className="button"
                  onClick={() => controller.current?.abort()}
                >
                  <X size={14} />
                  Cancel
                </button>
              ) : (
                <button
                  className="button primary"
                  disabled={!valid || !draft.fixture}
                  onClick={() => void run("test")}
                >
                  <Play size={14} />
                  Run simulation
                </button>
              )}
            </div>
          </div>
          <textarea
            className="fixture-editor"
            aria-label="JSONL observations"
            spellCheck={false}
            value={draft.fixture}
            onChange={(e) => {
              setDraft({ fixture: e.target.value, dirty: true });
              setResult(undefined);
            }}
            placeholder='One observation per line: {"sequence":1,"acceptedAt":"2026-01-01T00:00:00Z","health":"ok","fields":{"status":503}}'
          />
          {result && (
            <section className="panel simulation-result">
              <h2>
                {result.observations} observations → {result.events.length}{" "}
                predicted events
              </h2>
              <p className="muted">
                Simulation for {result.watchId}, revision{" "}
                {result.revision.slice(0, 12)}. First 100 events shown.
              </p>
              {result.events.slice(0, 100).map((e) => (
                <div className="entity-row" key={e.id}>
                  <Badge value={e.type} />
                  <Time value={e.at} />
                  <span>
                    {e.message} · inputs {e.evidence?.join(", ")}
                  </span>
                </div>
              ))}
              <Raw
                value={result}
                title="Full simulation report and resulting checkpoints"
                name="ding-simulation.json"
              />
            </section>
          )}
        </>
      )}
      {tab === "Verify evidence" && (
        <>
          <section className="panel">
            <h2>Verify recorded evidence</h2>
            <p>
              Reproduce a recorded event from its definition and checkpoint.
              This is separate from simulating a new definition.
            </p>
            <div className="editor-toolbar">
              {upload("evidence", 60 << 20)}
              <button
                className="button primary"
                disabled={!draft.evidence || busy}
                onClick={() => void run("replay")}
              >
                {busy ? "Verifying…" : "Verify recorded evidence"}
              </button>
            </div>
            <textarea
              className="fixture-editor"
              aria-label="Recorded evidence JSON"
              spellCheck={false}
              value={draft.evidence}
              onChange={(e) => {
                setDraft({ evidence: e.target.value, dirty: true });
                setVerification(undefined);
              }}
              placeholder="Paste an evidence object or import its JSON file…"
            />
            {verification && (
              <div className="verification" role="status">
                <Badge
                  value={
                    verification.status === "verified"
                      ? "verified"
                      : verification.status === "mismatch"
                        ? "error"
                        : "warning"
                  }
                >
                  {verification.status}
                </Badge>
                <p>{verification.message}</p>
              </div>
            )}
          </section>
        </>
      )}
    </>
  );
}
