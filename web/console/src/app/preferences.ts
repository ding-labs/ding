export function preference(key: string, fallback: string) {
  try {
    return localStorage.getItem(key) || fallback;
  } catch {
    return fallback;
  }
}
export function savePreference(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Preferences are optional when browser storage is unavailable. */
  }
}
export function savedViews(key: string): Record<string, string> {
  try {
    const v = JSON.parse(preference(key, "{}"));
    return Object.fromEntries(
      Object.entries(v)
        .filter(([, s]) => typeof s === "string")
        .slice(0, 20),
    ) as Record<string, string>;
  } catch {
    return {};
  }
}
