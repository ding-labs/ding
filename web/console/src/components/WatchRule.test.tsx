import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { StoreEntitySummary, WatchDefinition } from "../api/contracts";
import { WatchRule } from "./WatchRule";

function definition() {
  return {
    metadata: { id: "extraction" },
    spec: {
      source: { type: "http", url: "https://example.com/health", every: "5m" },
      condition: { numeric: "failureRate > 0.3" },
      policy: {
        trigger: "transition",
        consecutive: 3,
        recoverAfter: 2,
        onUnknown: "hold-incident",
      },
    },
  } as WatchDefinition;
}
const entity: StoreEntitySummary = {
  key: "[]",
  open: false,
  sourceUnhealthy: false,
  matches: 2,
  recoveries: 0,
  lastSequence: 4,
  samples: 0,
};
function show(d: WatchDefinition, e?: StoreEntitySummary, count = 1) {
  render(
    <MemoryRouter>
      <WatchRule definition={d} entity={e} entityCount={count} />
    </MemoryRouter>,
  );
}
describe("watch rule summary", () => {
  it("shows the live match count and switches to recovery progress for an open incident", () => {
    show(definition(), { ...entity, open: true, recoveries: 1 });
    const progress = screen.getByRole("progressbar", {
      name: "Recovery progress",
    });
    expect(progress).toHaveAttribute("value", "1");
    expect(progress).toHaveAttribute("max", "2");
    expect(
      screen.getByText("1 of 2 consecutive nonmatching checks to recover"),
    ).toBeVisible();
  });
  it("identifies retained progress while the source is unknown", () => {
    show(definition(), { ...entity, sourceUnhealthy: true });
    expect(
      screen.getByText("Source unknown · retained checkpoint"),
    ).toBeVisible();
    expect(
      screen.getByRole("progressbar", { name: "Consecutive matches" }),
    ).toHaveAttribute("value", "2");
  });
  it("does not invent progress for missing-input rules", () => {
    const d = definition();
    d.spec.condition = { missingFor: "15m", value: null };
    show(d, entity);
    expect(screen.getByText(/without an observation/)).toBeVisible();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });
  it.each(["new-event", "changed"])(
    "keeps %s semantics separate from incident recovery",
    (operator) => {
      const d = definition();
      d.spec.condition = {
        operator,
        field: "id",
        dedupFor: "24h",
        value: null,
      };
      d.spec.policy.trigger = "level";
      show(d, entity);
      expect(screen.queryByRole("progressbar")).toBeNull();
      expect(screen.queryByText(/Recover after/)).toBeNull();
      expect(screen.getByText("Event and source settings")).toBeVisible();
    },
  );
  it("links to per-entity progress without inventing a combined counter", () => {
    show(definition(), undefined, 8);
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(
      screen.getByRole("link", { name: "View 8 entities" }),
    ).toHaveAttribute("href", "/watches/extraction?tab=Entities");
  });
});
