import { test, expect } from "./fixtures";
import AxeBuilder from "@axe-core/playwright";

test("first useful watch is reviewed and observed without an account", async ({ page, daemon }) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Create your first watch" }).click();
  await expect(page.getByRole("heading", { name: "Your first useful watch" })).toBeVisible();
  await page.getByLabel("Watch ID").fill("local-health");
  await page.getByLabel("Health endpoint").fill(daemon.webhookURL);
  await page.getByLabel("Alert delivery").selectOption("console");
  await page.getByRole("button", { name: "Preview watch" }).click();
  await expect(page.getByRole("heading", { name: "Review before starting" })).toBeVisible();
  const before = await fetch(daemon.url + "/v1/watches", { headers: { Authorization: "Bearer " + daemon.token } });
  expect((await before.json()).data).toHaveLength(0);
  const a11y = await new AxeBuilder({ page }).analyze();
  expect(a11y.violations.filter(v => ["serious", "critical"].includes(v.impact ?? ""))).toEqual([]);
  await page.screenshot({ path: "/tmp/ding-first-watch.png", fullPage: true });
  await page.getByRole("button", { name: "Start this watch" }).click();
  await expect(page).toHaveURL(/watches\/local-health$/);
  await expect.poll(async () => {
    const r = await fetch(daemon.url + "/v1/doctor", { headers: { Authorization: "Bearer " + daemon.token } });
    const source = (await r.json()).data.sources.find((s: { id: string }) => s.id === "local-health");
    return !!source && !source.lastInputAt.startsWith("0001");
  }).toBeTruthy();
});

test("desktop onboarding requires visible-test confirmation and invalidates edits", async ({ page, daemon }) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Create your first watch" }).click();
  await page.getByLabel("Health endpoint").fill(daemon.webhookURL);
  await page.getByRole("button", { name: "Preview watch" }).click();
  await expect(page.getByRole("button", { name: "Start this watch" })).toBeDisabled();
  await page.route("**/v1/desktop/test", route => route.fulfill({ json: { apiVersion: "ding.ing/v1alpha1", data: { accepted: true } } }));
  await page.getByRole("button", { name: "Send a test notification" }).click();
  await page.getByLabel("I saw the test notification").check();
  await expect(page.getByRole("button", { name: "Start this watch" })).toBeEnabled();
  await page.getByLabel("Health endpoint").fill("http://localhost:3001/health");
  await expect(page.getByRole("button", { name: "Start this watch" })).toHaveCount(0);
});
