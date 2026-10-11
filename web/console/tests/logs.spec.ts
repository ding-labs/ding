import { test, expect } from "./fixtures";
import { call } from "./data";

test("draft, classify, apply, inspect and pause an external log watch", async ({
  page,
  daemon,
}) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page.getByRole("button", { name: "Log setup", exact: true }).click();
  await page.getByLabel("Window (seconds)").fill("1");
  await page.getByLabel("Minimum lines").fill("2");
  await page.getByRole("button", { name: "Preview classification" }).click();
  await expect(page.getByText("Record 3: unclassified")).toBeVisible();
  expect((await call(daemon, "/console/watches")).total).toBe(0);
  await page.screenshot({ path: "test-results/log-setup.png", fullPage: true });
  await page.getByRole("button", { name: "Create draft for review" }).click();
  await expect(
    page.getByText("Valid definition", { exact: false }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  await page.getByRole("button", { name: "Apply reviewed changes" }).click();
  await expect(
    page.getByRole("heading", { name: "Live log evidence" }),
  ).toBeVisible();
  await expect(
    page.getByText("Waiting for a collector", { exact: true }),
  ).toBeVisible();
  const session = await call(
    daemon,
    "/logs/application-logs/session",
    {},
    true,
  );
  await page.waitForTimeout(2100); // Complete a real one-second collection window.
  const end = Math.floor(Date.now() / 1000) * 1000;
  await call(
    daemon,
    "/logs/application-logs/summary",
    {
      session: session.id,
      sequence: 1,
      summary: {
        summary_version: 1,
        window_seconds: 1,
        window_start: new Date(end - 1000).toISOString(),
        window_end: new Date(end).toISOString(),
        total_count: 2,
        classified_count: 2,
        warning_count: 2,
        complete: true,
      },
    },
    true,
  );
  await page.reload();
  await expect(page.getByText(/100\.0% · 2 \/ 2 lines/)).toBeVisible({
    timeout: 10000,
  });
  await page.screenshot({
    path: "test-results/log-evidence.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Pause watch", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Pause watch", exact: true })
    .click();
  await expect
    .poll(
      async () =>
        (await call(daemon, "/watches/application-logs")).watch.status,
    )
    .toBe("paused");
});
