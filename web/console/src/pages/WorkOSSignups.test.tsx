import { afterEach, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WorkOSSignups } from "./WorkOSSignups";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.forEach((c) => c.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});
const response = (data: unknown) =>
  new Response(JSON.stringify({ apiVersion: "ding.ing/v1alpha1", data }));

function setup(lostResponse = false) {
  const requests: { path: string; body: any }[] = [];
  let connections: unknown[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, options) => {
    const path = String(input);
    if (!options?.method || options.method === "GET")
      return response(connections);
    const body = JSON.parse(String(options.body));
    requests.push({ path, body });
    if (path === "/v1/webhooks") {
      connections = [
        {
          id: "connection",
          name: body.name,
          provider: "workos",
          watch: body.watch,
        },
      ];
      if (lostResponse) throw new TypeError("Response lost");
      return response({
        connection: connections[0],
        url: "https://ding.example/v1/hooks/workos/connection",
      });
    }
    return response({ rotated: true });
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkOSSignups />
    </QueryClientProvider>,
  );
  return { requests, client, user: userEvent.setup() };
}

test("creates a signup connection and saves the signing secret without caching it", async () => {
  const { user, requests, client } = setup();
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Create signup connection" }),
    ).toBeEnabled(),
  );
  await user.click(
    screen.getByRole("button", { name: "Create signup connection" }),
  );
  expect(await screen.findByLabelText("WorkOS endpoint URL")).toHaveValue(
    `${window.location.origin}/v1/hooks/workos/connection`,
  );
  expect(requests[0].body).toMatchObject({
    provider: "workos",
    watch: "ding-account-signups",
    events: ["user.created"],
  });
  expect(requests[0].body.secret).toMatch(/^[a-f0-9]{64}$/);
  await user.type(
    screen.getByLabelText("WorkOS signing secret"),
    "whsec_private_signup_secret",
  );
  await user.click(
    screen.getByRole("button", { name: "Save WorkOS signing secret" }),
  );
  await screen.findByText(/Signing secret saved/);
  expect(requests[1]).toEqual({
    path: "/v1/webhooks/connection/rotate",
    body: { secret: "whsec_private_signup_secret", overlapSeconds: 0 },
  });
  expect(screen.getByLabelText("WorkOS signing secret")).toHaveValue("");
  expect(JSON.stringify(client.getQueryData(["/webhooks"]))).not.toContain(
    "whsec_private_signup_secret",
  );
  expect(
    screen.queryByRole("button", { name: "Create signup connection" }),
  ).not.toBeInTheDocument();
});

test("reconciles a lost creation response before allowing another connection", async () => {
  const { user, requests } = setup(true);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Create signup connection" }),
    ).toBeEnabled(),
  );
  await user.click(
    screen.getByRole("button", { name: "Create signup connection" }),
  );
  await screen.findByLabelText("WorkOS endpoint URL");
  expect(requests).toHaveLength(1);
  expect(
    screen.queryByRole("button", { name: "Create signup connection" }),
  ).not.toBeInTheDocument();
});
