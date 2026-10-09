import { test, expect, type Daemon } from "./fixtures";
export const manifest = `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: console}
spec: {type: console}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api-health, name: API health}
spec:
  source: {type: push}
  condition: {field: status, operator: gte, value: 500}
  policy: {trigger: transition, consecutive: 3, recoverAfter: 2}
  destinations: [{ref: console, events: [firing, recovered]}]
`;
export async function call(
  daemon: Daemon,
  path: string,
  body?: unknown,
  ingest = false,
) {
  const r = await fetch(daemon.url + "/v1" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Authorization: "Bearer " + (ingest ? daemon.ingest : daemon.token),
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const out = await r.json();
  if (!r.ok) throw new Error(out.error?.message);
  return out.data;
}
test("real observations explain a firing and delivery; list filters distinguish empty states", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  for (let i = 0; i < 3; i++)
    await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await page.goto(await daemon.launch());
  await expect(page.getByRole("link", { name: "API health" })).toBeVisible();
  await expect(
    page.getByText("1 incident open", { exact: true }),
  ).toBeVisible();
  await page.getByLabel("Search watches").fill("unmatched");
  await expect(
    page.getByRole("heading", { name: "No watches match this view" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clear filters" }).click();
  await page.getByRole("link", { name: "API health" }).click();
  await expect(
    page.getByRole("heading", { name: "Incident opened", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Replay verified", { exact: false }),
  ).toBeVisible();
  await expect(page.getByText("503", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Inspect event" }).click();
  await expect(page.getByText("Prior matching count")).toBeVisible();
  await page
    .getByRole("button", { name: "Inspect retained supporting observations" })
    .click();
  await expect(
    page.getByRole("button", { name: "Hide retained supporting observations" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: /console.*Delivered/ }),
  ).toBeVisible();
  await page.getByRole("link", { name: /console.*Delivered/ }).click();
  await expect(
    page.getByRole("heading", { name: "Attempt history" }),
  ).toBeVisible();
});
