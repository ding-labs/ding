import { test, expect, type Daemon } from "./fixtures";
import { manifest, call } from "./data";
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
