import { test, expect } from "./fixtures";
import { call, manifest } from "./data";
test("validate, simulate, review and apply a push watch", async ({
  page,
  daemon,
}) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "New watch", exact: true }).click();
  await page.getByRole("button", { name: /Receive a signal/ }).click();
  await expect(
    page.getByText("Valid definition", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Simulation", exact: true }).click();
  await page.getByRole("button", { name: "Run simulation" }).click();
  await expect(
    page.getByRole("heading", { name: "5 observations → 2 predicted events" }),
  ).toBeVisible();
  expect((await call(daemon, "/console/watches")).total).toBe(0);
  await page
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Understand the change. Then apply it.",
    }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "A new watch starts running after this transaction commits.",
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "Apply reviewed changes" }).click();
  await expect(
    page.getByRole("heading", { name: "Pushed status", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Waiting for first input", { exact: true }),
  ).toBeVisible();
});
test("invalid draft and destination conflicts retain edits and require new review", async ({
  page,
  daemon,
}) => {
  await call(daemon, "/apply", { manifest });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await page.getByLabel("Manifest", { exact: true }).fill("kind: Wrong");
  await expect(page.getByRole("alert")).toContainText("invalid_manifest");
  await expect(
    page.getByRole("button", { name: "Review changes", exact: true }),
  ).toBeDisabled();
  await page
    .getByLabel("Manifest", { exact: true })
    .fill(manifest.replace("consecutive: 3", "consecutive: 4"));
  await expect(
    page.getByText("Valid definition", { exact: false }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Apply reviewed changes" }),
  ).toBeVisible();
  await call(daemon, "/apply", {
    manifest: manifest.replace(
      "spec: {type: console}",
      "spec: {type: console, maxAttempts: 2}",
    ),
  });
  await page.getByRole("button", { name: "Apply reviewed changes" }).click();
  await expect(
    page.getByRole("button", { name: "Compare with current" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Compare with current" }).click();
  await expect(
    page.getByRole("button", { name: "Compare with current" }),
  ).not.toBeVisible();
  await page.getByRole("button", { name: "Back to editor" }).click();
  await expect(page.getByLabel("Manifest", { exact: true })).toHaveValue(
    /consecutive: 4/,
  );
  await page.getByRole("link", { name: "Watches", exact: true }).click();
  await page.getByRole("link", { name: "Workbench", exact: true }).click();
  await expect(page.getByLabel("Manifest", { exact: true })).toHaveValue(
    /consecutive: 4/,
  );
});
for (const kind of ["HTTP", "Command"]) {
  test(`create and simulate ${kind} source through the UI`, async ({
    page,
    daemon,
  }) => {
    await page.goto(await daemon.launch());
    await page.getByRole("link", { name: "Workbench", exact: true }).click();
    await page.getByLabel("Choose example").selectOption(kind);
    if (kind === "HTTP") {
      const field = page.getByLabel("Manifest", { exact: true });
      await field.fill(
        (await field.inputValue()).replace(
          "https://example.com/health",
          daemon.webhookURL,
        ),
      );
    }
    await expect(
      page.getByText("Valid definition", { exact: false }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Simulation", exact: true }).click();
    await page.getByRole("button", { name: "Run simulation" }).click();
    await expect(
      page.getByRole("heading", {
        name: "5 observations → 2 predicted events",
      }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Review changes", exact: true })
      .click();
    await page.getByRole("button", { name: "Apply reviewed changes" }).click();
    const id = kind === "HTTP" ? "api-health" : "local-check";
    await expect(page).toHaveURL(new RegExp(`/watches/${id}$`));
    await expect
      .poll(
        async () => (await call(daemon, `/watches/${id}`)).watch.lastInputAt,
      )
      .not.toMatch(/^0001/);
    expect(
      (await call(daemon, `/watches/${id}`)).watch.plan.definition.spec.source
        .type,
    ).toBe(kind.toLowerCase());
  });
}
