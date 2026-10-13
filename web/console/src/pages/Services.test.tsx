import { afterEach, expect, test, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ExecutionContext } from "../app/execution";
import { Services } from "./Services";

const producer = {
  id: "producer",
  name: "push-soak-producer-20261013-rerun",
  scopes: ["ingest"],
  watches: ["ci"],
  expiresAt: "2099-10-18T18:00:00Z",
};
const clients: QueryClient[] = [];
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});
const response = (data: unknown) =>
  new Response(JSON.stringify({ apiVersion: "ding.ing/v1alpha1", data }));
function setup({ supported = true, createError = false } = {}) {
  const user = userEvent.setup();
  const requests: { path: string; method: string; body: any }[] = [];
  let rows = [producer];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, options) => {
    const path = String(input),
      method = options?.method || "GET",
      body = options?.body ? JSON.parse(String(options.body)) : undefined;
    requests.push({ path, method, body });
    if (path === "/v1/info")
      return response({ push: { supported, admissionOpen: true } });
    if (path === "/v1/services" && method === "GET") return response(rows);
    if (path === "/v1/services" && method === "POST") {
      if (createError) throw new TypeError("Response lost");
      const created = { ...producer, ...body, id: "new-service" };
      rows = [...rows, created];
      return response({ ...created, token: "ding_service_test_secret" });
    }
    throw new Error(`Unexpected request: ${method} ${path}`);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <ExecutionContext.Provider value={{ mode: "cloud" }}>
        <Services />
      </ExecutionContext.Provider>
    </QueryClientProvider>,
  );
  return { user, requests, client };
}

test("creation groups permissions and expiry, and the secret survives accidental dismissal", async () => {
  const { user, requests, client } = setup();
  await screen.findByText(producer.name, { exact: false });
  expect(
    screen.queryByLabelText("Credential name", { exact: true }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Add credential" }));
  const dialog = within(screen.getByRole("dialog", { name: "Add credential" }));
  await user.type(
    dialog.getByLabelText("Credential name", { exact: true }),
    "  GitHub Actions  ",
  );
  await user.selectOptions(dialog.getByLabelText("Permission"), "ingest");
  await user.type(
    dialog.getByLabelText("Allowed push watch IDs", { exact: true }),
    "ci, deploy, ci, ",
  );
  await user.selectOptions(dialog.getByLabelText("Key expires in"), "custom");
  await user.clear(dialog.getByLabelText("Expiry in days"));
  await user.type(dialog.getByLabelText("Expiry in days"), "17");
  await user.click(dialog.getByRole("button", { name: "Create credential" }));
  await screen.findByRole("dialog", { name: "Credential created" });
  expect(requests.filter((r) => r.method === "POST")).toEqual([
    {
      path: "/v1/services",
      method: "POST",
      body: {
        name: "GitHub Actions",
        scopes: ["ingest"],
        watches: ["ci", "deploy"],
        days: 17,
      },
    },
  ]);
  expect(screen.getByLabelText("New service credential")).toHaveValue(
    "ding_service_test_secret",
  );
  expect(screen.getByRole("button", { name: "Done" })).toBeDisabled();
  await user.keyboard("{Escape}");
  expect(
    screen.getByRole("dialog", { name: "Credential created" }),
  ).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Copy key" }));
  await screen.findByRole("button", { name: "Copied" });
  expect(await navigator.clipboard.readText()).toBe("ding_service_test_secret");
  expect(JSON.stringify(client.getQueryData(["/services"]))).not.toContain(
    "ding_service_test_secret",
  );
  await user.click(screen.getByLabelText("I’ve saved this key"));
  await user.click(screen.getByRole("button", { name: "Done" }));
  expect(
    screen.queryByLabelText("New service credential"),
  ).not.toBeInTheDocument();
});

test("a lost issuance response keeps the form and explains how to recover", async () => {
  const { user, requests } = setup({ createError: true });
  await user.click(screen.getByRole("button", { name: "Add credential" }));
  await user.type(
    screen.getByLabelText("Credential name", { exact: true }),
    "Inspector",
  );
  await user.click(screen.getByRole("button", { name: "Create credential" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "The key may have been created",
  );
  expect(screen.getByLabelText("Credential name", { exact: true })).toHaveValue(
    "Inspector",
  );
  expect(
    screen.queryByLabelText("New service credential"),
  ).not.toBeInTheDocument();
  await waitFor(() =>
    expect(
      requests.filter((r) => r.path === "/v1/services" && r.method === "GET")
        .length,
    ).toBeGreaterThan(1),
  );
  expect(requests.filter((r) => r.method === "POST")).toHaveLength(1);
});

test("unsupported push access is omitted and clipboard failures allow manual copying", async () => {
  const { user } = setup({ supported: false });
  await screen.findByText(producer.name, { exact: false });
  await user.click(screen.getByRole("button", { name: "Add credential" }));
  expect(
    screen.queryByRole("option", { name: "Send input to selected watches" }),
  ).not.toBeInTheDocument();
  await user.type(
    screen.getByLabelText("Credential name", { exact: true }),
    "Inspector",
  );
  await user.click(screen.getByRole("button", { name: "Create credential" }));
  await screen.findByRole("dialog", { name: "Credential created" });
  vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(
    new Error("Denied"),
  );
  await user.click(screen.getByRole("button", { name: "Copy key" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "copy it manually",
  );
  await user.click(screen.getByRole("button", { name: "Reveal key" }));
  expect(screen.getByLabelText("New service credential")).toHaveAttribute(
    "type",
    "text",
  );
});
