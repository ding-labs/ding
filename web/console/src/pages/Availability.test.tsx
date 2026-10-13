import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Availability } from "./Availability";
import { ExecutionContext } from "../app/execution";
const { read } = vi.hoisted(() => ({ read: vi.fn() }));
vi.mock("../components/common", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../components/common")>()),
  useRead: read,
}));
beforeEach(() => read.mockReturnValue({ data: [], error: null }));
describe("execution disclosure", () => {
  it("keeps hosting instructions collapsed", () => {
    render(
      <ExecutionContext.Provider value={{ mode: "cloud" }}>
        <Availability watchID="extraction" />
      </ExecutionContext.Provider>,
    );
    expect(screen.getByText("Execution and hosting")).toBeVisible();
    expect(
      screen.getByRole("heading", {
        name: "Where this watch runs",
        hidden: true,
      }),
    ).not.toBeVisible();
  });
  it("keeps transfer holds visible even while hosting instructions are collapsed", () => {
    read.mockReturnValue({
      data: [
        {
          id: "transfer-1",
          watchId: "extraction",
          role: "source",
          phase: "prepared",
          held: true,
        },
      ],
      error: null,
    });
    render(<Availability watchID="extraction" />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Execution is held by a transfer",
    );
    expect(screen.getByRole("status")).toBeVisible();
    expect(document.querySelector("details")).not.toHaveAttribute("open");
  });
});
