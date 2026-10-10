import { createContext, useContext } from "react";

export type Execution = { mode: "local" | "cloud"; workspace?: string };
export const ExecutionContext = createContext<Execution>({ mode: "local" });
export const useExecution = () => useContext(ExecutionContext);

export async function discoverCloud(): Promise<boolean> {
  try {
    const response = await fetch("/v1/cloud/config", { credentials: "same-origin" });
    if (!response.ok) return false;
    const result = await response.json();
    return result.apiVersion === "ding.ing/v1alpha1" && result.data?.execution === "cloud";
  } catch { return false; }
}
