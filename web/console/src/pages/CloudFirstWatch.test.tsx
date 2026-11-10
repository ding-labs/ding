import { afterEach, expect, test, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { CloudFirstWatch } from "./CloudFirstWatch";

afterEach(() => vi.restoreAllMocks());

test.each(["accepted", "unreadable", "missing result", "unknown version"])("lost delivery-test replies reuse their key and activation handles %s replies", async (reply) => {
  const tests: string[] = [];
  const applies: unknown[] = [];
  const response = (data: unknown) => new Response(JSON.stringify({ apiVersion: "ding.ing/v1alpha1", data }), { status: 200 });
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, options) => {
    const path = String(input);
    const body = options?.body ? JSON.parse(String(options.body)) : {};
    if (path === "/v1/cloud/secrets") return response({ names: ["DING_WEBHOOK"] });
    if (path.startsWith("/v1/console/destinations?")) return response({ destinations: [], more: false, cursor: "" });
    if (path === "/v1/onboarding/preview") return response({ manifest: "exact reviewed manifest", descriptions: [{ id: "first-cloud-watch", source: "Check every 7m (timeout 4s).", condition: "http.status does not equal 200.", policy: "Open after 4 consecutive matches." }], review: { review: { manifestHash: "review" } } });
    if (path === "/v1/cloud/destination-test") {
      tests.push(body.operationKey);
      if (tests.length === 1) throw new TypeError("response lost");
      return response({ outcome: "accepted" });
    }
    if (path === "/v1/apply") {
      applies.push(body);
      if (reply === "unreadable") return new Response("truncated");
      if (reply === "missing result") return response(undefined);
      if (reply === "unknown version") return new Response(JSON.stringify({apiVersion: "future", data: {}}));
      return response({});
    }
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><CloudFirstWatch /></MemoryRouter></QueryClientProvider>);
  await screen.findByText(/credential is saved/);
  fireEvent.change(screen.getByLabelText("Public health endpoint"), { target: { value: "https://example.com/health" } });
  fireEvent.click(screen.getByRole("button", { name: "Preview cloud watch" }));
  await screen.findByText("Review hosted execution");
  expect(screen.getByText("Check every 7m (timeout 4s).")).toBeVisible();
  expect(screen.getByText("http.status does not equal 200.")).toBeVisible();
  expect(screen.getByText("Open after 4 consecutive matches.")).toBeVisible();
  expect(screen.getByRole("button", { name: "Start in Ding Cloud" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Send labeled delivery test" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Check test outcome" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Check test outcome" }));
  const confirmation = await screen.findByLabelText("I received the test at my destination");
  expect(tests).toHaveLength(2);
  expect(tests[0]).toBe(tests[1]);
  expect(screen.getByRole("button", { name: "Start in Ding Cloud" })).toBeDisabled();
  fireEvent.click(confirmation);
  fireEvent.click(screen.getByRole("button", { name: "Start in Ding Cloud" }));
  if (reply !== "accepted") {
    expect(await screen.findByRole("link", {name: "this watch"})).toHaveAttribute("href", "/watches/first-cloud-watch");
    expect(screen.getByRole("button", {name: "Start in Ding Cloud"})).toBeDisabled();
  }
  await waitFor(() => expect(applies).toEqual([{ manifest: "exact reviewed manifest", review: { manifestHash: "review" }, dryRun: false }]));
});

test("saved hosted delivery loads more destinations and needs no new credentials", async () => {
  const requests: { path: string; body: Record<string, unknown> }[] = [];
  const response = (data: unknown) => new Response(JSON.stringify({ apiVersion: "ding.ing/v1alpha1", data }), { status: 200 });
  const destination = { definition: { metadata: { id: "team-alerts", name: "Team alerts" }, spec: { type: "slack" } }, revision: "listed-revision", watches: 2 };
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, options) => {
    const path = String(input);
    const body = options?.body ? JSON.parse(String(options.body)) : {};
    if (options?.method === "POST") requests.push({ path, body });
    if (path === "/v1/cloud/secrets" && options?.method !== "POST") return response({ names: [] });
    if (path.startsWith("/v1/console/destinations?")) return response(path.endsWith("cursor=next") ? { destinations: [destination], more: false, cursor: "" } : { destinations: [], more: true, cursor: "next" });
    if (path === "/v1/onboarding/preview") return response({ manifest: "reference-only-watch", descriptions: [], review: { review: { manifestHash: "review", destinations: { "team-alerts": "reviewed-revision" } } } });
    if (path === "/v1/cloud/destination-test") return response({ outcome: "accepted" });
    if (path === "/v1/apply") return response({});
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><CloudFirstWatch /></MemoryRouter></QueryClientProvider>);
  fireEvent.click(await screen.findByRole("button", { name: "Load more destinations" }));
  await screen.findByRole("option", { name: "Team alerts (slack)" });
  fireEvent.change(screen.getByLabelText("Saved destination"), { target: { value: "team-alerts" } });
  expect(screen.queryByLabelText("Destination webhook URL")).toBeNull();
  fireEvent.change(screen.getByLabelText("Public health endpoint"), { target: { value: "https://example.com/health" } });
  fireEvent.click(screen.getByRole("button", { name: "Preview cloud watch" }));
  await screen.findByText("Review hosted execution");
  fireEvent.click(screen.getByRole("button", { name: "Send saved destination test" }));
  const confirmation = await screen.findByLabelText("I received the test at this destination");
  expect(screen.getByRole("button", { name: "Start in Ding Cloud" })).toBeDisabled();
  fireEvent.click(confirmation);
  fireEvent.click(screen.getByRole("button", { name: "Start in Ding Cloud" }));
  await waitFor(() => expect(requests.some(r => r.path === "/v1/apply")).toBe(true));
  expect(requests.find(r => r.path === "/v1/onboarding/preview")?.body).toEqual({ id: "first-cloud-watch", url: "https://example.com/health", destinationRef: "team-alerts" });
  expect(requests.find(r => r.path === "/v1/cloud/destination-test")?.body).toEqual({ operationKey: expect.any(String), destinationRef: "team-alerts", revision: "reviewed-revision" });
  expect(requests.some(r => r.path === "/v1/cloud/secrets")).toBe(false);
});
