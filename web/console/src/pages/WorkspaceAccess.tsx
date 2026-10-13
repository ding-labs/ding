import { useState } from "react";
import { ErrorBox, useRead } from "../components/common";
type AuditRow = {
  sequence: number;
  actor: string;
  action: string;
  target: string;
  at: string;
};
export function Audit() {
  const [source, setSource] = useState("runtime");
  const [before, setBefore] = useState(0);
  const records = useRead<AuditRow[]>(
    `/audit?source=${source}&before=${before}`,
  );
  return (
    <section className="panel">
      <h2>Workspace audit</h2>
      <p>
        Committed changes are recorded with their actor. Passwords, tokens, and
        credential values are excluded. Each store retains its latest 10,000
        records.
      </p>
      <label>
        History
        <select
          value={source}
          onChange={(e) => {
            setSource(e.target.value);
            setBefore(0);
          }}
        >
          <option value="runtime">Watches and operations</option>
          <option value="identity">Members and credentials</option>
        </select>
      </label>
      <ErrorBox error={records.error} />
      <ul>
        {records.data?.map((a) => (
          <li key={a.sequence}>
            {new Date(a.at).toLocaleString()} · {a.actor} · {a.action}{" "}
            <code>{a.target}</code>
          </li>
        ))}
      </ul>
      <button
        className="button"
        disabled={!before}
        onClick={() => setBefore(0)}
      >
        Latest
      </button>{" "}
      <button
        className="button"
        disabled={records.data?.length !== 100}
        onClick={() => setBefore(records.data?.at(-1)?.sequence || 0)}
      >
        Older
      </button>
    </section>
  );
}
