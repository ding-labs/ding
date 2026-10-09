import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
test("pause, resume and delete expose committed state and keep history", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health" }).click();
  await page.getByRole("button", { name: "Pause watch", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText(
    "Committed deliveries continue",
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Pause watch", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Resume watch", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Resume watch", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Resume watch", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Pause watch", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "More watch actions" }).click();
  await page.getByRole("menuitem", { name: "Delete watch…" }).click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "Delete watch", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Watch ID to delete").fill("api-health");
  await page.getByLabel("Cancel pending deliveries").check();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Delete watch", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect((await call(daemon, "/watches/api-health")).watch.status).toBe(
    "deleted",
  );
  expect(
    (await call(daemon, "/console/events?watch=api-health")).events.length,
  ).toBeGreaterThan(2);
});
test("terminal delivery retry keeps original identity and records a new policy cycle", async ({
  page,
  daemon,
}) => {
  const m = manifest.replace(
    "spec: {type: console}",
    "spec: {type: webhook, urlRef: {env: DING_TEST_WEBHOOK_URL}}",
  );
  await call(daemon, "/apply", { manifest: m });
  for (let i = 0; i < 3; i++)
    await call(daemon, "/ingest/api-health", { status: 503 }, true);
  await expect
    .poll(
      async () =>
        (await call(daemon, "/console/deliveries")).deliveries[0]?.status,
    )
    .toBe("permanent");
  const original = (await call(daemon, "/console/deliveries")).deliveries[0];
  await page.goto(await daemon.launch());
  await page.goto(`${daemon.url}/ui/deliveries/${original.id}`);
  await page
    .getByRole("button", { name: "Retry delivery", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("duplicate");
  await page
    .getByRole("button", { name: "Retry original notification" })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(page.getByText("new delivery policy cycle")).toBeVisible();
  const current = await call(daemon, `/deliveries/${original.id}`);
  expect(current.intent.eventId).toBe(original.eventId);
  expect(current.intent.destinationRevision).toBe(original.destinationRevision);
  expect(
    current.attempts.filter(
      (a: { outcome: string }) => a.outcome === "manual_retry",
    ),
  ).toHaveLength(1);
});
