import { afterEach, expect, test, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { LogPreview } from "./LogPreview";

afterEach(() => vi.restoreAllMocks());

test("sample edits invalidate a previous classification", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ apiVersion: "ding.ing/v1alpha1", data: { total: 3, classified: 2, warnings: 1, errors: 0, invalid: 0, levels: ["warning", "info", "unclassified"] } }), { status: 200 }));
  render(<LogPreview config={{ input: { type: "external" }, format: "auto" }} />);
  fireEvent.click(screen.getByRole("button", { name: "Preview classification" }));
  expect(await screen.findByText("Record 3: unclassified")).toBeVisible();
  fireEvent.change(screen.getByLabelText("Sample output"), { target: { value: "different sample" } });
  expect(screen.queryByText("Record 3: unclassified")).toBeNull();
});
