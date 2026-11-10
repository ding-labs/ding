import type { StoreLifecycleGuidance } from "../api/contracts";

// Known actions double as the fallback for servers predating detail guidance.
const byStatus = new Map<string, string[]>([
  ["running", ["pause", "delete"]],
  ["paused", ["resume", "delete"]],
  ["deleted", ["purge"]],
]);

export function watchActions(status: string, guidance?: StoreLifecycleGuidance | null) {
  const known = byStatus.get(status) ?? [];
  return guidance ? known.filter(action => guidance.actions?.includes(action)) : known;
}
