import { afterEach, expect, test, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { CloudFirstWatch } from "./CloudFirstWatch";

afterEach(() => vi.restoreAllMocks());

test("a lost delivery-test response reuses its key and activation requires confirmation", async () => {
  const tests: string[] = [];
  const applies: unknown[] = [];
  const response = (data: unknown) => new Response(JSON.stringify({ apiVersion: "ding.ing/v1alpha1", data }), { status: 200 });
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, options) => {
    const path = String(input);
    const body = options?.body ? JSON.parse(String(options.body)) : {};
    if (path === "/v1/cloud/secrets") return response({ names: ["DING_WEBHOOK"] });
    if (path === "/v1/onboarding/preview") return response({ manifest: "exact reviewed manifest", review: { review: { manifestHash: "review" } } });
    if (path === "/v1/cloud/destination-test") {
      tests.push(body.operationKey);
      if (tests.length === 1) throw new TypeError("response lost");
      return response({ outcome: "accepted" });
    }
    if (path === "/v1/apply") { applies.push(body); return response({}); }
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><CloudFirstWatch /></MemoryRouter></QueryClientProvider>);
  await screen.findByText(/credential is saved/);
  fireEvent.change(screen.getByLabelText("Public health endpoint"), { target: { value: "https://example.com/health" } });
  fireEvent.click(screen.getByRole("button", { name: "Preview cloud watch" }));
  await screen.findByText("Review hosted execution");
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
  await waitFor(() => expect(applies).toEqual([{ manifest: "exact reviewed manifest", review: { manifestHash: "review" }, dryRun: false }]));
});
