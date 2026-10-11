import { afterEach, expect, it, vi } from "vitest";
import { api, forgetSession, selectWorkspace } from "./client";
import { discoverAuth } from "../app/execution";
const response = (data: unknown) => new Response(JSON.stringify({apiVersion: "ding.ing/v1alpha1", data}), {status: 200});
afterEach(() => { forgetSession(); vi.restoreAllMocks(); });
it("binds each request to the selected workspace and rejects old responses", async () => {
  let finish!: (value: Response) => void;
  const fetch = vi.spyOn(globalThis, "fetch").mockImplementation(() => new Promise(resolve => {finish = resolve;}));
  selectWorkspace("first");
  const pending = api("/watches");
  const checked = expect(pending).rejects.toMatchObject({ name: "AbortError" });
  expect(fetch.mock.calls[0][1]?.headers).toMatchObject({ "X-Ding-Workspace": "first" });
  selectWorkspace("second"); finish(response({secret: "first workspace"}));
  await checked;
});
it("does not interpret failed discovery as personal access", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("unavailable", {status:503}));
  await expect(discoverAuth()).rejects.toThrow();
});
