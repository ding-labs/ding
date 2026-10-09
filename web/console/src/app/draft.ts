import { useSyncExternalStore } from "react";
type Draft = {
  manifest: string;
  fixture: string;
  evidence: string;
  legacy: string;
  dirty: boolean;
};
let value: Draft = {
  manifest: "",
  fixture: "",
  evidence: "",
  legacy: "",
  dirty: false,
};
const listeners = new Set<() => void>();
export function setDraft(patch: Partial<Draft>) {
  value = { ...value, ...patch };
  listeners.forEach((fn) => fn());
}
export function useDraft() {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn);
      return () => {
        listeners.delete(fn);
      };
    },
    () => value,
  );
}
window.addEventListener("ding:draft", (e) =>
  setDraft({ manifest: (e as CustomEvent<string>).detail, dirty: true }),
);
window.addEventListener("beforeunload", (e) => {
  if (value.dirty) {
    e.preventDefault();
    e.returnValue = "";
  }
});
