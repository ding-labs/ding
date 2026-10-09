import { test, expect, type Daemon } from "./fixtures";
import { execFileSync } from "node:child_process";
import { manifest, call } from "./data";
test("real observations explain a firing and delivery; list filters distinguish empty states", async ({
  page,
  daemon,
  binary,
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
  await page.getByLabel("Search watches").fill("API health");
  await page.getByRole("link", { name: "API health" }).click();
  await expect(
    page.getByRole("heading", { name: "Incident opened", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Replay verified", { exact: false }),
  ).toBeVisible();
  await expect(page.getByText("503", { exact: true })).toBeVisible();
  await expect(
    page.getByText(/This input matched the condition/),
  ).toBeVisible();
  await page.getByRole("link", { name: "Watches", exact: true }).last().click();
  await expect(page.getByLabel("Search watches")).toHaveValue("API health");
  await expect(
    page.getByRole("link", { name: "API health", exact: true }),
  ).toBeFocused();
  await page.getByRole("link", { name: "API health", exact: true }).click();
  const exported = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export", exact: true }).click();
  const exportFile = await (await exported).path();
  expect(
    JSON.parse(
      execFileSync(binary, ["validate", exportFile!, "--json"], {
        encoding: "utf8",
      }),
    ).error,
  ).toBeUndefined();
  await page.getByRole("link", { name: "Inspect event" }).click();
  const evidence = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download evidence", exact: true })
    .click();
  const proofFile = await (await evidence).path();
  expect(
    JSON.parse(
      execFileSync(binary, ["replay", proofFile!, "--json"], {
        encoding: "utf8",
      }),
    ).data.verified,
  ).toBe(true);
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
