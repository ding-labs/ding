import { useSyncExternalStore } from "react";
let available = navigator.onLine;
const subscribers = new Set<() => void>();
export function setConnection(value: boolean) {
  value = value && navigator.onLine;
  if (available !== value) {
    available = value;
    subscribers.forEach((fn) => fn());
  }
}
window.addEventListener("offline", () => setConnection(false));
export function useConnection() {
  return useSyncExternalStore(
    (fn) => {
      subscribers.add(fn);
      return () => {
        subscribers.delete(fn);
      };
    },
    () => available,
  );
}
