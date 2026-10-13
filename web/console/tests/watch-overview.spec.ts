import { test, expect } from "./fixtures";
import { call, manifest } from "./data";

test("overview separates the latest input from configuration activity and exposes rule progress", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "No open incidents", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("progressbar", { name: "Consecutive matches" }),
  ).toHaveAttribute("value", "1");
  await expect(
    page.getByRole("heading", { name: "Latest configuration activity" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Watch created", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Why the latest event happened")).toHaveCount(0);
  await page.getByText("Latest input details", { exact: true }).click();
  await expect(
    page.getByRole("link", { name: "View entity state" }),
  ).toBeVisible();
  await page.getByText("Recovery and source settings", { exact: true }).click();
  await expect(
    page.getByText("Recover after 2 consecutive inputs that do not match."),
  ).toBeVisible();
  await page.getByRole("link", { name: "Inspect event", exact: true }).click();
  await expect(
    page.getByText("No condition evaluation", { exact: true }),
  ).toBeVisible();
});
