import { ErrorBox, Time, useRead } from "./common";
type Status = {
  lastAcceptance?: {
    acceptanceId: string;
    acceptedAt: string;
    first: number;
    last: number;
  };
  recent: { requestId: string; at: string; status: number; code: string }[];
  note: string;
};
export function PushStatus({ watch }: { watch: string }) {
  const status = useRead<Status>(
    `/ingest-status/${encodeURIComponent(watch)}`,
    5000,
  );
  return (
    <div>
      <h3>Input acceptance</h3>
      <ErrorBox error={status.error} />
      {status.data && (
        <>
          {status.data.lastAcceptance ? (
            <p>
              Last durable acceptance:{" "}
              <Time value={status.data.lastAcceptance.acceptedAt} /> ·
              observations {status.data.lastAcceptance.first}–
              {status.data.lastAcceptance.last}. Check Events and Deliveries for
              the monitoring and notification outcomes.
            </p>
          ) : (
            <p>Waiting for the first accepted input.</p>
          )}
          <details>
            <summary>Recent input requests</summary>
            <p>{status.data.note}</p>
            <ul>
              {status.data.recent
                .slice()
                .reverse()
                .map((r) => (
                  <li key={r.requestId}>
                    <Time value={r.at} /> · HTTP {r.status} {r.code} · request{" "}
                    <code>{r.requestId}</code>
                  </li>
                ))}
            </ul>
          </details>
        </>
      )}
    </div>
  );
}
