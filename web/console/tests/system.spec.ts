import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
import { readFileSync } from "node:fs";
test("doctor, destinations, help, completion and verified backup", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "System", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Storage integrity" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Credentials on the daemon host" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Destinations", exact: true }).click();
  await expect(
    page.getByRole("link", { name: "1 watches reference this destination" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "CLI setup", exact: true }).click();
  await page
    .getByRole("button", { name: "ding watch pause", exact: true })
    .click();
  await expect(page.locator(".command-block")).toContainText(
    "expected-revision",
  );
  const completion = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download completion" }).click();
  expect((await completion).suggestedFilename()).toBe("ding.zsh");
  await page.getByRole("button", { name: "Backup", exact: true }).click();
  const backup = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download verified backup", exact: true })
    .click();
  const d = await backup;
  const file = await d.path();
  expect(file).toBeTruthy();
  expect(readFileSync(file!).subarray(0, 15).toString()).toBe(
    "SQLite format 3",
  );
  await expect(page.getByRole("status")).toContainText("Verified");
});
test("legacy import returns a report and archive without applying", async ({
  page,
  daemon,
}) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page
    .getByRole("button", { name: "Legacy import", exact: true })
    .click();
  await page
    .getByLabel("Legacy YAML", { exact: true })
    .fill(
      `notifiers:\n  console: {type: console}\nrules:\n  - name: hot\n    condition: value > 5\n    alert: [{notifier: console}]\n`,
    );
  await page
    .getByRole("button", { name: "Convert legacy configuration" })
    .click();
  await expect(
    page.getByRole("heading", { name: "1 converted · 0 unsupported" }),
  ).toBeVisible();
  expect((await call(daemon, "/console/watches")).total).toBe(0);
  const archive = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download conversion archive" })
    .click();
  expect((await archive).suggestedFilename()).toBe("ding-migration.zip");
  await page.getByRole("button", { name: "Open in Workbench" }).click();
  await expect(page.getByLabel("Manifest", { exact: true })).toContainText(
    "kind: Watch",
  );
});
