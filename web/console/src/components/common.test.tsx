import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Badge, Raw, Rule, Time, TimeContext, ErrorBox } from "./common";
import { DefinitionDiff } from "./DefinitionDiff";
import type { WatchDefinition } from "../api/contracts";
import { APIError } from "../api/client";
describe("evidence presentation", () => {
  it("distinguishes unknown, paused and pending states without relying on color alone", () => {
    render(
      <>
        <Badge value="unknown" />
        <Badge value="paused" />
        <Badge value="pending" />
      </>,
    );
    expect(screen.getByText("Unknown")).toHaveClass("unknown");
    expect(screen.getByText("Paused")).toHaveClass("neutral");
    expect(screen.getByText("Queued / retry scheduled")).toHaveClass("warn");
  });
  it("never implies incident recovery for a deduplicated provider event", () => {
    const d = {
      spec: {
        source: { type: "push" },
        condition: { operator: "new-event", field: "id", dedupFor: "24h" },
        policy: {
          trigger: "level",
          consecutive: 1,
          recoverAfter: 1,
          onUnknown: "hold-incident",
        },
      },
    } as WatchDefinition;
    render(<Rule definition={d} />);
    expect(screen.getByText(/Emit a new-event event/)).toBeVisible();
    expect(screen.queryByText(/recover after/i)).toBeNull();
  });
  it("renders untrusted content as text and bounds large JSON", () => {
    render(
      <Raw value={{ payload: "<img src=x onerror=alert(1)>".repeat(1000) }} />,
    );
    expect(document.querySelector("img")).toBeNull();
    const detail = document.querySelector("details")!;
    detail.open = true;
    fireEvent(detail, new Event("toggle"));
    expect(screen.getByText(/Preview limited to 16,000/)).toBeVisible();
    expect(document.querySelector("pre")!.textContent!.length).toBeLessThan(
      16200,
    );
  });
  it("exposes missing timestamps instead of a fake date", () => {
    render(<Time value="0001-01-01T00:00:00Z" />);
    expect(screen.getByText("Not yet")).toBeVisible();
  });
  it("keeps absolute timestamps available in relative mode", () => {
    render(
      <TimeContext.Provider value="relative">
        <Time value="2026-01-01T00:00:00Z" />
      </TimeContext.Provider>,
    );
    expect(document.querySelector("time")).toHaveAttribute(
      "dateTime",
      "2026-01-01T00:00:00Z",
    );
    expect(document.querySelector("time")!.title).toContain("2026-01-01");
  });
  it("makes retention discontinuity explicit", () => {
    render(
      <ErrorBox
        error={new APIError("cursor_expired", "History is gone", 410)}
        retry={() => {}}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Retained history has a gap",
    );
    expect(
      screen.getByRole("button", { name: "Load available history" }),
    ).toBeVisible();
  });
  it("labels actual definition differences without rendering HTML", () => {
    render(<DefinitionDiff before={"a: 1\nb: 2"} after={"a: 1\nb: 3"} />);
    const d = document.querySelector("details")!;
    d.open = true;
    expect(screen.getByLabelText("Definition changes")).toHaveTextContent(
      "− b: 2",
    );
    expect(screen.getByLabelText("Definition changes")).toHaveTextContent(
      "+ b: 3",
    );
  });
});
