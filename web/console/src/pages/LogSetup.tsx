import { useState } from "react";
import type { StoreDestinationPage, WatchLogSource } from "../api/contracts";
import { ErrorBox, useRead } from "../components/common";
import { LogPreview } from "../components/LogPreview";

export function LogSetup({ onDraft }: { onDraft: (manifest: string) => void }) {
  const [input, setInput] = useState("external");
  const [id, setID] = useState("application-logs");
  const [path, setPath] = useState("/tmp/application.log");
  const [argv, setArgv] = useState(
    '["docker", "logs", "--follow", "--tail", "0", "my-app"]',
  );
  const [directory, setDirectory] = useState("/tmp");
  const [format, setFormat] = useState("auto");
  const [field, setField] = useState("level");
  const [window, setWindow] = useState(60);
  const [minimum, setMinimum] = useState(100);
  const [coverage, setCoverage] = useState(0);
  const [threshold, setThreshold] = useState(50);
  const [metric, setMetric] = useState("warning_match_ratio");
  const [destination, setDestination] = useState("");
  const [error, setError] = useState<unknown>();
  const destinations = useRead<StoreDestinationPage>(
    "/console/destinations?limit=100",
  );
  const config: WatchLogSource = {
    input: { type: input },
    format,
    levelField: field,
    framing: "line",
    window: `${window}s`,
    minEvents: minimum,
    minCoverage: coverage / 100,
    retention: "1h",
  };
  return (
    <>
      <form
        className="panel log-setup"
        onSubmit={(e) => {
          e.preventDefault();
          setError(undefined);
          try {
            if (input === "file")
              config.input = { type: input, path, start: "end" };
            if (input === "command") {
              const args: unknown = JSON.parse(argv);
              if (
                !Array.isArray(args) ||
                !args.length ||
                !args.every((a) => typeof a === "string")
              )
                throw new Error("Enter a JSON array of command arguments.");
              config.input = {
                type: input,
                argv: args,
                directory,
                reconnectEvery: "5s",
              };
            }
            const manifest = JSON.stringify(
              {
                apiVersion: "ding.ing/v1alpha1",
                kind: "Watch",
                metadata: { id },
                spec: {
                  source: { type: "logs", logs: config },
                  condition: {
                    field: metric,
                    operator: "gt",
                    value: threshold / 100,
                  },
                  policy: {
                    trigger: "transition",
                    consecutive: 1,
                    recoverAfter: 3,
                    onUnknown: "hold-incident",
                    maxGap: "5s",
                  },
                  destinations: [
                    {
                      ref: destination || "log-console",
                      events: [
                        "firing",
                        "recovered",
                        "source_error",
                        "source_recovered",
                      ],
                    },
                  ],
                },
              },
              null,
              2,
            );
            const sink = destination
              ? ""
              : "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: log-console}\nspec: {type: console}\n---\n";
            onDraft(sink + manifest);
          } catch (e) {
            setError(e);
          }
        }}
      >
        <h2>Monitor application output</h2>
        <p>
          Plain text and JSON work on every input. Ratios divide detected
          matches by all nonblank lines, including unclassified output. Stderr
          alone does not mean error.
        </p>
        <div className="log-fields">
          <label>
            Watch ID
            <input
              required
              value={id}
              onChange={(e) => setID(e.target.value)}
            />
          </label>
          <label>
            Input
            <select value={input} onChange={(e) => setInput(e.target.value)}>
              <option value="external">
                Launch application or pipe output
              </option>
              <option value="file">Follow a file in the daemon</option>
              <option value="command">Run a log reader in the daemon</option>
            </select>
          </label>
          {input === "file" && (
            <label>
              Absolute file path
              <input
                required
                value={path}
                onChange={(e) => setPath(e.target.value)}
              />
            </label>
          )}
          {input === "command" && (
            <>
              <label>
                Command arguments (JSON array)
                <input
                  required
                  value={argv}
                  onChange={(e) => setArgv(e.target.value)}
                />
              </label>
              <label>
                Absolute working directory
                <input
                  required
                  value={directory}
                  onChange={(e) => setDirectory(e.target.value)}
                />
              </label>
            </>
          )}
          <label>
            Format
            <select value={format} onChange={(e) => setFormat(e.target.value)}>
              <option value="auto">Auto: JSON or text headers</option>
              <option value="text">Text headers</option>
              <option value="jsonl">One JSON object per line</option>
            </select>
          </label>
          <label>
            JSON severity field
            <input value={field} onChange={(e) => setField(e.target.value)} />
          </label>
          <label>
            Metric
            <select value={metric} onChange={(e) => setMetric(e.target.value)}>
              <option value="warning_match_ratio">Detected warnings</option>
              <option value="error_match_ratio">
                Detected errors or fatal
              </option>
            </select>
          </label>
          <label>
            Above percentage
            <input
              type="number"
              min={0}
              max={100}
              step="any"
              required
              value={threshold}
              onChange={(e) => setThreshold(e.target.valueAsNumber)}
            />
          </label>
          <label>
            Window (seconds)
            <input
              type="number"
              min={1}
              max={3600}
              required
              value={window}
              onChange={(e) => setWindow(e.target.valueAsNumber)}
            />
          </label>
          <label>
            Minimum lines
            <input
              type="number"
              min={1}
              required
              value={minimum}
              onChange={(e) => setMinimum(e.target.valueAsNumber)}
            />
          </label>
          <label>
            Minimum classification coverage (%)
            <input
              type="number"
              min={0}
              max={100}
              required
              value={coverage}
              onChange={(e) => setCoverage(e.target.valueAsNumber)}
            />
          </label>
          <label>
            Destination
            <select
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
            >
              <option value="">Console output (create log-console)</option>
              {destinations.data?.destinations.map((d) => (
                <option
                  key={d.definition.metadata.id}
                  value={d.definition.metadata.id}
                >
                  {d.definition.metadata.name || d.definition.metadata.id}
                </option>
              ))}
            </select>
          </label>
        </div>
        <p>
          {input === "external"
            ? "Applying this watch waits for a foreground capture session. Start it with ding logs exec or ding logs ingest; capture ends when that session ends."
            : input === "file"
              ? "The daemon reads new output from the current end. Rotation and skipped backlog produce a visible gap."
              : "Applying launches the configured observer on the daemon host. Use a live-only log reader, with explicit environment references if needed. It reconnects after exit; pausing stops the observer."}
        </p>
        <p>
          Three ready, nonmatching summaries recover an incident. Quiet or
          incomplete windows hold it open. Ordinary summaries are retained for
          one hour; incident evidence may remain longer. Customize patterns,
          multiline framing, and environment references in the draft.
        </p>
        <ErrorBox error={error || destinations.error} />
        <button className="button primary" type="submit">
          Create draft for review
        </button>
      </form>
      <LogPreview config={config} />
    </>
  );
}
