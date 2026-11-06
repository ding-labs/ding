import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { expect, test } from "vitest";
import type { StoreConsoleWatch } from "../api/contracts";
import { FirstWatchProgress } from "./FirstWatchProgress";

function fixture(overrides: Partial<StoreConsoleWatch> = {}): StoreConsoleWatch {
  return { id: "api", name: "api", revision: "r", status: "running", source: "http", trigger: "transition", operator: "gte", generation: 1, nextAt: "2026-11-05T23:00:00Z", lastInputAt: "0001-01-01T00:00:00Z", lastError: "", entities: 0, open: 0, unhealthy: 0, failed: 0, pending: 0, missing: [], ...overrides };
}

test("waits for actual monitoring input and offers another watch after observation", () => {
  const { rerender } = render(<MemoryRouter><FirstWatchProgress watch={fixture()} /></MemoryRouter>);
  expect(screen.getByRole("status")).toHaveTextContent("Waiting for the first observation");
  expect(screen.queryByRole("link", { name: "Create another watch" })).toBeNull();
  expect(screen.getByText(/Next scheduled check/)).toBeVisible();
  rerender(<MemoryRouter><FirstWatchProgress watch={fixture({ lastInputAt: "2026-11-05T22:55:00Z", entities: 1, open: 1 })} /></MemoryRouter>);
  expect(screen.getByRole("status")).toHaveTextContent("Monitoring input received");
  expect(screen.getByText(/not service health or notification delivery/)).toBeVisible();
  expect(screen.getByRole("link", { name: "Create another watch" })).toHaveAttribute("href", "/start");
  expect(screen.getByRole("link", { name: "Inspect evidence" })).toHaveAttribute("href", "/watches/api?tab=Events");
});

test.each([
  { lastError: "timeout" },
  { unhealthy: 1 },
  { missing: ["HTTP_TOKEN"] },
])("does not count unknown inputs as a successful check: %j", (problem) => {
  render(<MemoryRouter><FirstWatchProgress watch={fixture({ lastInputAt: "2026-11-05T22:55:00Z", entities: 1, ...problem })} /></MemoryRouter>);
  expect(screen.getByRole("status")).toHaveTextContent("Monitoring needs attention");
  expect(screen.queryByRole("link", { name: "Create another watch" })).toBeNull();
});

test("keeps notification failures separate from received observations", () => {
  render(<MemoryRouter><FirstWatchProgress watch={fixture({ lastInputAt: "2026-11-05T22:55:00Z", entities: 1, failed: 1, pending: 2 })} /></MemoryRouter>);
  expect(screen.getByRole("status")).toHaveTextContent("Monitoring input received");
  expect(screen.getByText(/could not be delivered/)).toBeVisible();
  expect(screen.getByText(/queued or being sent/)).toBeVisible();
  expect(screen.getByRole("link", { name: "Inspect deliveries" })).toHaveAttribute("href", "/watches/api?tab=Deliveries");
});

test("does not claim monitoring continues after a pause or unavailable status", () => {
  const { rerender } = render(<MemoryRouter><FirstWatchProgress watch={fixture({ status: "paused", lastInputAt: "2026-11-05T22:55:00Z", entities: 1 })} /></MemoryRouter>);
  expect(screen.getByRole("status")).toHaveTextContent("Monitoring is stopped");
  expect(screen.queryByRole("link", { name: "Create another watch" })).toBeNull();
  rerender(<MemoryRouter><FirstWatchProgress /></MemoryRouter>);
  expect(screen.getByText(/Saving the watch does not confirm a successful check/)).toBeVisible();
  expect(screen.queryByRole("link", { name: "Create another watch" })).toBeNull();
});
