import { test, expect } from "./fixtures";
import { call, manifest } from "./data";

test("permanent deletion requires confirmation and removes the watch across reloads", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await call(daemon, "/apply", {
    manifest: manifest
      .replaceAll("api-health", "other-watch")
      .replaceAll("API health", "Other watch"),
  });
  await call(daemon, "/watches/api-health/lifecycle", { action: "delete" });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Pause watch", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Permanently delete…", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("deletes its recorded history");
  await expect(dialog).toContainText("cannot be undone");
  const confirm = dialog.getByRole("button", {
    name: "Permanently delete watch",
    exact: true,
  });
  await expect(confirm).toBeDisabled();
  await page.getByLabel("Watch ID to delete").fill("wrong-id");
  await expect(confirm).toBeDisabled();
  await dialog.getByRole("button", { name: "Keep current state" }).click();
  expect((await call(daemon, "/watches/api-health")).watch.status).toBe(
    "deleted",
  );
  await page
    .getByRole("button", { name: "Permanently delete…", exact: true })
    .click();
  await expect(page.getByLabel("Watch ID to delete")).toHaveValue("");
  await page.getByLabel("Watch ID to delete").fill("api-health");
  await confirm.click();
  await expect(page).toHaveURL(/\/ui\/watches$/);
  await expect(
    page.getByRole("link", { name: "Other watch", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "API health", exact: true }),
  ).toHaveCount(0);
  await page.reload();
  await expect(
    page.getByRole("link", { name: "Other watch", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "API health", exact: true }),
  ).toHaveCount(0);
  expect((await call(daemon, "/console/watches")).watches).toHaveLength(1);
  expect(
    (await call(daemon, "/console/events?watch=api-health")).events,
  ).toHaveLength(0);
});

test("a lost permanent-delete response is reconciled without resubmission", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await call(daemon, "/watches/api-health/lifecycle", { action: "delete" });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health", exact: true }).click();
  let writes = 0;
  await page.route("**/v1/watches/api-health/lifecycle", async (route) => {
    writes++;
    await route.fetch();
    await route.abort("connectionreset");
  });
  await page
    .getByRole("button", { name: "Permanently delete…", exact: true })
    .click();
  await page.getByLabel("Watch ID to delete").fill("api-health");
  await page
    .getByRole("button", { name: "Permanently delete watch", exact: true })
    .click();
  await expect(page.getByText("The outcome is unknown.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Permanently delete watch", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Inspect current state" }).click();
  await expect(page).toHaveURL(/\/ui\/watches$/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(writes).toBe(1);
  expect(await call(daemon, "/watches")).toHaveLength(0);
});

test("a rejected permanent deletion keeps the watch and shows the error", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await call(daemon, "/watches/api-health/lifecycle", { action: "delete" });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "API health", exact: true }).click();
  await page.route("**/v1/watches/api-health/lifecycle", async (route) => {
    const body = route.request().postDataJSON();
    // Exercise the real API's concurrency check, rather than mocking its error.
    await route.continue({
      postData: JSON.stringify({
        ...body,
        expectedGeneration: body.expectedGeneration - 1,
      }),
    });
  });
  await page
    .getByRole("button", { name: "Permanently delete…", exact: true })
    .click();
  await page.getByLabel("Watch ID to delete").fill("api-health");
  await page
    .getByRole("button", { name: "Permanently delete watch", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "expected revision does not match",
  );
  await expect(page).toHaveURL(/\/ui\/watches\/api-health$/);
  expect((await call(daemon, "/watches/api-health")).watch.status).toBe(
    "deleted",
  );
});
