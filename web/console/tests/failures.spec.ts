import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
test("unknown lifecycle outcome is reconciled without automatic resubmission", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health", exact: true }).click();
  let writes = 0;
  await page.route("**/v1/watches/api-health/lifecycle", async (route) => {
    writes++;
    await route.fetch();
    await route.abort("connectionreset");
  });
  await page.getByRole("button", { name: "Pause watch", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Pause watch", exact: true })
    .click();
  await expect(page.getByText("The outcome is unknown.")).toBeVisible();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "Pause watch", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Inspect current state" }).click();
  await expect(
    page.getByRole("button", { name: "Resume watch", exact: true }),
  ).toBeVisible();
  expect(writes).toBe(1);
});
test("offline keeps last good data and disables writes", async ({
  page,
  daemon,
  context,
}) => {
  await call(daemon, "/apply", { manifest });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Pause watch", exact: true }),
  ).toBeVisible();
  await context.setOffline(true);
  await expect(
    page.getByRole("button", { name: "Pause watch", exact: true }),
  ).toBeDisabled({ timeout: 10000 });
  await expect(
    page.getByRole("heading", { name: "API health", exact: true }),
  ).toBeVisible();
  await expect(page.getByText(/Displayed data may be stale/)).toBeVisible();
  await context.setOffline(false);
  await expect(
    page.getByRole("button", { name: "Pause watch", exact: true }),
  ).toBeEnabled({ timeout: 20000 });
});
test("event follow announces new records without replacing history", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Events", exact: true }).click();
  await page.getByRole("button", { name: "Follow live" }).click();
  for (let i = 0; i < 3; i++)
    await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await expect(
    page.getByRole("button", { name: /1 new event.*Show newest/ }),
  ).toBeVisible({ timeout: 10000 });
  await expect(page.locator(".event-row")).toHaveCount(1);
  await page.getByRole("button", { name: /1 new event.*Show newest/ }).click();
  await expect(page.locator(".event-row")).toHaveCount(2);
  await page.getByRole("button", { name: "Pause live updates" }).click();
  expect((await call(daemon, "/watches/api-health")).watch.status).toBe(
    "running",
  );
});
test("an unacknowledged retry is inspected before another cycle is allowed", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", {
    manifest: manifest.replace(
      "spec: {type: console}",
      "spec: {type: webhook, urlRef: {env: DING_TEST_WEBHOOK_URL}}",
    ),
  });
  for (let i = 0; i < 3; i++)
    await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await expect
    .poll(
      async () =>
        (await call(daemon, "/console/deliveries")).deliveries[0]?.status,
    )
    .toBe("permanent");
  const d = (await call(daemon, "/console/deliveries")).deliveries[0];
  await page.goto(await daemon.launch());
  // A second navigation must not cancel the asynchronous login exchange.
  await expect(page.getByRole("link", { name: "Watches", exact: true })).toBeVisible();
  await page.goto(`${daemon.url}/ui/deliveries/${d.id}`);
  let writes = 0;
  await page.route(`**/v1/deliveries/${d.id}/retry`, async (r) => {
    writes++;
    await r.fetch();
    await r.abort("connectionreset");
  });
  await page
    .getByRole("button", { name: "Retry delivery", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Retry original notification" })
    .click();
  await expect(page.getByText("The outcome is unknown.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Retry original notification" }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Inspect current state" }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect(writes).toBe(1);
  expect(
    (await call(daemon, `/deliveries/${d.id}`)).attempts.filter(
      (a: { outcome: string }) => a.outcome === "manual_retry",
    ),
  ).toHaveLength(1);
});
test("an unacknowledged apply compares committed revisions before resubmission", async ({
  page,
  daemon,
}) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page.getByLabel("Manifest", { exact: true }).fill(manifest);
  await expect(
    page.getByText("Valid definition", { exact: false }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  let commits = 0;
  await page.route("**/v1/apply", async (r) => {
    if (r.request().postDataJSON().dryRun) {
      await r.continue();
      return;
    }
    commits++;
    await r.fetch();
    await r.abort("connectionreset");
  });
  await page.getByRole("button", { name: "Apply reviewed changes" }).click();
  await expect(page.getByText("The outcome is unknown.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Apply reviewed changes" }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Reconcile current definitions" })
    .click();
  await expect(
    page.getByText("Existing condition state is preserved."),
  ).toBeVisible();
  expect(commits).toBe(1);
});
