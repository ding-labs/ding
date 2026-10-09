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
export async function api<T>(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const method = options.method || "GET";
  const res = await fetch("/v1" + path, {
    method,
    credentials: "same-origin",
    signal: options.signal,
    headers: {
      "Content-Type": "application/json",
      ...(csrf ? { "X-Ding-CSRF": csrf } : {}),
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
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
