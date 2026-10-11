import { expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { LogEvidence } from "./LogEvidence";
import type { WatchObservation } from "../api/contracts";

test("an incomplete window never displays zero percent as evidence of recovery", () => {
  const observation: WatchObservation = { sequence: 1, inputId: "example", acceptedAt: new Date().toISOString(), fields: { reason: "insufficient_records", ready: false, total_count: 0, warning_match_ratio: null, error_match_ratio: null }, health: "waiting" };
  render(<LogEvidence id="app" status="running" config={{ input: { type: "external" }, window: "60s", minEvents: 100 }} observation={observation} />);
  expect(screen.getByText("Waiting for enough output")).toBeVisible();
  expect(screen.queryByText(/0\.0%/)).toBeNull();
  expect(screen.getByText(/Quiet or incomplete windows hold/)).toBeVisible();
});
