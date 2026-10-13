import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { StoreConsoleWatch, StoreWatchSummary } from "../api/contracts";
import { WatchContext, WatchStatus } from "./WatchStatus";
import { ExecutionContext } from "../app/execution";

function fixture() {
  const detail = {
    watch: {
      status: "running",
      nextAt: "2026-10-13T21:09:27Z",
      lastInputAt: "2026-10-13T21:04:24Z",
      plan: {
        definition: {
          metadata: { id: "extraction" },
          spec: {
            source: { type: "http", every: "5m0s" },
            condition: { numeric: "failureRate > 0.3" },
            policy: { trigger: "transition", consecutive: 3, recoverAfter: 2 },
            destinations: [
              { ref: "ding-slack", events: ["firing", "recovered"] },
            ],
          },
        },
      },
    },
    entities: [
      {
        key: "[]",
        open: false,
        matches: 0,
        recoveries: 0,
        lastSequence: 4109,
        sourceUnhealthy: false,
        samples: 0,
      },
    ],
    entitiesMore: false,
  } as StoreWatchSummary;
  const summary = {
    entities: 1,
    open: 0,
    unhealthy: 0,
    failed: 0,
    pending: 0,
    missing: [],
  } as unknown as StoreConsoleWatch;
  return { detail, summary };
}

function show({
  detail,
  summary,
  error = null,
  loading = false,
}: {
  detail: StoreWatchSummary;
  summary?: StoreConsoleWatch;
  error?: unknown;
  loading?: boolean;
}) {
  render(
    <MemoryRouter>
      <WatchStatus
        detail={detail}
        summary={summary}
        error={error}
        loading={loading}
        retry={vi.fn()}
      />
    </MemoryRouter>,
  );
}

describe("watch status overview", () => {
  it.each(["local", "shared", "cloud"] as const)(
    "describes the cadence and %s execution host",
    (mode) => {
      render(
        <ExecutionContext.Provider value={{ mode }}>
          <WatchContext definition={fixture().detail.watch.plan.definition} />
        </ExecutionContext.Provider>,
      );
      expect(screen.getByText(/Every 5 minutes/)).toHaveTextContent(
        {
          local: "This computer",
          shared: "Shared server",
          cloud: "Ding Cloud",
        }[mode],
      );
    },
  );
  it("leads with the condition and schedule independently of recorded events", () => {
    show(fixture());
    expect(
      screen.getByRole("heading", { name: "No open incidents" }),
    ).toBeVisible();
    expect(
      screen.getByText("0 consecutive matches at the current checkpoint", {
        exact: false,
      }),
    ).toBeVisible();
    expect(screen.getByText("Next scheduled check")).toBeVisible();
    expect(
      document.querySelector('time[datetime="2026-10-13T21:09:27Z"]'),
    ).toBeVisible();
  });

  it("does not present stale totals as a clear condition or zero failed deliveries", () => {
    show({ ...fixture(), error: new Error("Connection lost") });
    expect(
      screen.getByRole("heading", { name: "Condition unavailable" }),
    ).toBeVisible();
    expect(
      screen.getByRole("heading", { name: "Delivery status unavailable" }),
    ).toBeVisible();
    expect(screen.queryByText("No open incidents")).toBeNull();
    expect(screen.queryByText("failed or pending")).toBeNull();
    expect(screen.getByRole("alert")).toHaveTextContent("Connection lost");
  });

  it("keeps open incidents visible while suppressing a paused watch's old schedule", () => {
    const data = fixture();
    data.detail.watch.status = "paused";
    data.summary.open = 1;
    show(data);
    expect(screen.getByRole("heading", { name: "Watch paused" })).toBeVisible();
    expect(screen.getByText(/1 open incident/)).toBeVisible();
    expect(screen.getByText("Checks stopped")).toBeVisible();
    expect(
      document.querySelector('time[datetime="2026-10-13T21:09:27Z"]'),
    ).toBeNull();
  });

  it("does not imply that no incidents means the source is healthy", () => {
    const data = fixture();
    data.summary.unhealthy = 1;
    show(data);
    expect(
      screen.getByRole("heading", { name: "Source needs attention" }),
    ).toBeVisible();
    expect(
      screen.getByRole("heading", { name: "Acquisition error" }),
    ).toBeVisible();
    expect(screen.getByText(/0 open incidents/)).toBeVisible();
    expect(screen.queryByText("No acquisition error reported")).toBeNull();
  });

  it("describes pushed event watches without a polling schedule or incident claim", () => {
    const data = fixture();
    data.detail.watch.plan.definition.spec.source.type = "push";
    data.detail.watch.plan.definition.spec.condition.operator = "new-event";
    data.detail.watch.plan.definition.spec.policy.trigger = "level";
    show(data);
    expect(
      screen.getByRole("heading", { name: "Watching for new events" }),
    ).toBeVisible();
    expect(screen.getByText("When your source sends an input")).toBeVisible();
    expect(screen.queryByText("Next scheduled check")).toBeNull();
    expect(screen.queryByText("No open incidents")).toBeNull();
  });

  it("does not aggregate per-entity progress into a misleading single counter", () => {
    const data = fixture();
    data.summary.entities = 8;
    data.detail.entitiesMore = true;
    show(data);
    expect(screen.getByText("8 monitored entities")).toBeVisible();
    expect(
      screen.queryByText(/consecutive matches at the current checkpoint/),
    ).toBeNull();
  });
});
