import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { Workspace } from "./Workspace";
import type { Bridge, ToolResult } from "./bridge";

afterEach(cleanup);
const result = (view: string, data: Record<string, unknown>): ToolResult => ({
  structuredContent: { view, data },
});
function mockBridge(initial: ToolResult, call?: Bridge["call"]): Bridge {
  return {
    async connect(receive) {
      receive(initial);
    },
    call: vi.fn(async (name, args) =>
      name === "ding_get_capabilities"
        ? result("capabilities", {
            grant: { scopes: ["inspect", "preview", "manage"] },
          })
        : call
          ? call(name, args)
          : result("watches", {
              watches: [],
              total: 0,
              more: false,
              cursor: "",
            }),
    ),
  };
}
const preview = result("preview", {
  valid: true,
  preview: {
    handle: "a".repeat(64),
    expiresAt: "2099-01-01T00:00:00Z",
    changes: {
      changes: [
        {
          kind: "Watch",
          id: "api",
          state: "created",
          revision: "b".repeat(64),
          permissions: [],
          after: "source: push",
        },
      ],
      destinationChanges: [],
      credentials: [],
      review: {},
      dryRun: true,
    },
  },
});

test("preview renders before an explicit apply; retries retain the operation key", async () => {
  const bridge = mockBridge(preview, async () => ({
    isError: true,
    content: [
      { type: "text", text: "outcome_unknown: check the original key" },
    ],
  }));
  render(<Workspace bridge={bridge} />);
  const apply = await screen.findByRole("button", {
    name: "Apply reviewed changes",
  });
  await waitFor(() =>
    expect((apply as HTMLButtonElement).disabled).toBe(false),
  );
  expect(
    vi
      .mocked(bridge.call)
      .mock.calls.filter((c) => c[0] === "ding_apply_changes"),
  ).toHaveLength(0);
  fireEvent.click(apply);
  fireEvent.click(apply);
  await screen.findByRole("alert");
  const first = vi
    .mocked(bridge.call)
    .mock.calls.find((c) => c[0] === "ding_apply_changes")!;
  fireEvent.click(screen.getByRole("button", { name: "Retry same request" }));
  await waitFor(() =>
    expect(
      vi
        .mocked(bridge.call)
        .mock.calls.filter((c) => c[0] === "ding_apply_changes"),
    ).toHaveLength(2),
  );
  const attempts = vi
    .mocked(bridge.call)
    .mock.calls.filter((c) => c[0] === "ding_apply_changes");
  expect(attempts[1][1]).toEqual(first[1]);
  await waitFor(() =>
    expect((apply as HTMLButtonElement).disabled).toBe(false),
  );
  fireEvent.click(apply);
  await waitFor(() =>
    expect(
      vi
        .mocked(bridge.call)
        .mock.calls.filter((c) => c[0] === "ding_apply_changes"),
    ).toHaveLength(3),
  );
  expect(
    vi
      .mocked(bridge.call)
      .mock.calls.filter((c) => c[0] === "ding_apply_changes")[2][1],
  ).toEqual(first[1]);
});

test("event messages are rendered as text, never HTML", async () => {
  const hostile = '<img src=x onerror="window.pwned=true">';
  render(
    <Workspace
      bridge={mockBridge(
        result("events", {
          events: [
            {
              id: "e",
              at: "2026-10-10T00:00:00Z",
              watchId: "api",
              type: "firing",
              message: hostile,
            },
          ],
          more: false,
        }),
      )}
    />,
  );
  expect(await screen.findByText(hostile)).toBeTruthy();
  expect(document.querySelector("img")).toBeNull();
});

test("expired previews cannot apply", async () => {
  const initial = structuredClone(preview);
  (
    initial.structuredContent as { data: { preview: { expiresAt: string } } }
  ).data.preview.expiresAt = "2000-01-01T00:00:00Z";
  render(<Workspace bridge={mockBridge(initial)} />);
  await screen.findByText("Review your changes");
  expect(
    (
      screen.getByRole("button", {
        name: "Apply reviewed changes",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});
