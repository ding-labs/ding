import { setConnection } from "./connection";
import type { ControlBrowserSession } from "./contracts";
export class APIError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}
let csrf = "";

// Consume bounded NDJSON progress from a request-scoped operation. Aborting the
// request stops its temporary runtime on the daemon as well.
export async function stream<T>(path: string, body: unknown, signal: AbortSignal, receive: (value: T) => void): Promise<void> {
  const response = await fetch("/v1" + path, {
    method: "POST", credentials: "same-origin", signal,
    headers: { "Content-Type": "application/json", ...(csrf ? { "X-Ding-CSRF": csrf } : {}) },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    const result = await response.json();
    if (response.status === 401) window.dispatchEvent(new Event("ding:unauthorized"));
    throw new APIError(result.error?.code || "request_failed", result.error?.message || "Ding could not start this operation.", response.status);
  }
  if (!response.body || !response.headers.get("Content-Type")?.includes("application/x-ndjson")) throw new Error("Ding returned an unreadable progress stream.");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffered = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      buffered += decoder.decode(value, { stream: !done });
      if (buffered.length > 1 << 20) throw new Error("Ding's progress response exceeded its limit.");
      const lines = buffered.split("\n");
      buffered = lines.pop() || "";
      for (const line of lines) if (line.trim()) receive(JSON.parse(line) as T);
      if (done) break;
    }
    if (buffered.trim()) receive(JSON.parse(buffered) as T);
  } finally { await reader.cancel(); reader.releaseLock(); }
}
export async function api<T>(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const method = options.method || "GET";
  let res: Response;
  try {
    res = await fetch("/v1" + path, {
      method,
      credentials: "same-origin",
      signal: options.signal,
      headers: {
        "Content-Type": "application/json",
        ...(csrf ? { "X-Ding-CSRF": csrf } : {}),
      },
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch (error) {
    if (!options.signal?.aborted) setConnection(false);
    throw error;
  }
  let result: {
    apiVersion?: string;
    data?: T;
    error?: { code: string; message: string };
  };
  try {
    result = await res.json();
  } catch {
    throw new APIError(
      "invalid_response",
      "Ding returned an unreadable response.",
      res.status,
    );
  }
  if (!res.ok || result.error) {
    if (
      res.status === 401 ||
      result.error?.code === "shutting_down" ||
      result.error?.code === "store_unavailable"
    )
      setConnection(false);
    if (res.status === 401)
      window.dispatchEvent(new Event("ding:unauthorized"));
    throw new APIError(
      result.error?.code || "request_failed",
      result.error?.message || "Ding could not complete this request.",
      res.status,
    );
  }
  if (result.apiVersion !== "ding.ing/v1alpha1")
    throw new APIError(
      "version_mismatch",
      "This console and daemon use different API versions.",
      res.status,
    );
  setConnection(true);
  return result.data as T;
}
export async function connect(): Promise<ControlBrowserSession> {
  const hash = new URLSearchParams(location.hash.slice(1));
  const token = hash.get("handoff");
  // Remove the one-time secret even when exchange fails.
  if (token)
    history.replaceState(null, "", location.pathname + location.search);
  const session = await api<ControlBrowserSession>(
    "/browser/session",
    token ? { method: "POST", body: { token } } : {},
  );
  csrf = session.csrf;
  return session;
}
export async function logout() {
  await api("/browser/session", { method: "DELETE" });
  csrf = "";
}
export function download(
  name: string,
  data: BlobPart,
  type = "application/json",
) {
  const url = URL.createObjectURL(new Blob([data], { type }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function unknownOutcome(error: unknown) {
  return (
    !(error instanceof APIError) ||
    error.status >= 500 ||
    error.code === "invalid_response"
  );
}
