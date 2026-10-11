import { createContext, useContext } from "react";
import { api } from "../api/client";
import type { ControlBrowserSession } from "../api/contracts";

export type Execution = { mode: "local" | "shared" | "cloud"; workspace?: string; user?: ControlBrowserSession["user"]; role?: string; capabilities?: string[] };
export type AuthConfig = { access: "personal" | "shared"; execution: Execution["mode"]; name?: string; methods: { id: string; label: string; url?: string }[] };
export const ExecutionContext = createContext<Execution>({ mode: "local" });
export const useExecution = () => useContext(ExecutionContext);
export const useCapability = (action: string) => { const e = useExecution(); return e.mode === "local" || Boolean(e.capabilities?.includes(action)); };
export async function discoverAuth(): Promise<AuthConfig> {
  const config = await api<AuthConfig>("/auth/config");
  if (!["personal", "shared"].includes(config.access) || !["local", "shared", "cloud"].includes(config.execution) || !Array.isArray(config.methods)) throw new Error("This server returned an invalid authentication configuration.");
  return config;
}
