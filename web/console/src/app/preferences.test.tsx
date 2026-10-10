import { afterEach, expect, test } from "vitest";
import { preference, savePreference, setPreferenceWorkspace } from "./preferences";

afterEach(() => { setPreferenceWorkspace(""); localStorage.clear(); });

test("saved views are isolated across hosted workspaces and local use", () => {
  savePreference("ding.views", "local");
  setPreferenceWorkspace("workspace-a");
  expect(preference("ding.views", "empty")).toBe("empty");
  savePreference("ding.views", "private-a");
  setPreferenceWorkspace("workspace-b");
  expect(preference("ding.views", "empty")).toBe("empty");
  setPreferenceWorkspace("workspace-a");
  expect(preference("ding.views", "empty")).toBe("private-a");
  setPreferenceWorkspace("");
  expect(preference("ding.views", "empty")).toBe("local");
});
