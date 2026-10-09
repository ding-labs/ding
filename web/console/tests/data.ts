import type { Daemon } from "./fixtures";
export const manifest = `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: console}
spec: {type: console}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api-health, name: API health}
spec:
  source: {type: push}
  condition: {field: status, operator: gte, value: 500}
  policy: {trigger: transition, consecutive: 3, recoverAfter: 2}
  destinations: [{ref: console, events: [firing, recovered]}]
`;
export async function call(
  daemon: Daemon,
  path: string,
  body?: unknown,
  ingest = false,
) {
  const r = await fetch(daemon.url + "/v1" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Authorization: "Bearer " + (ingest ? daemon.ingest : daemon.token),
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const out = await r.json();
  if (!r.ok) throw new Error(out.error?.message);
  return out.data;
}
