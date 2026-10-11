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
  await page.getByRole("button", { name: "Send test notification", exact: true }).click();
  await page.getByRole("button", { name: "Done", exact: true }).click();
  await page.getByLabel("I saw the test notification").check();
  await expect(page.getByRole("button", { name: "Start this watch" })).toBeEnabled();
  await page.getByLabel("Health endpoint").fill("http://localhost:3001/health");
  await expect(page.getByRole("button", { name: "Start this watch" })).toHaveCount(0);
});

test("notification setup explains helper failure and opens OS settings", async ({ page, daemon }) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "System", exact: true }).click();
  await page.route("**/v1/desktop/status", route => route.fulfill({ json: { apiVersion: "ding.ing/v1alpha1", data: { platform: "darwin", appName: "Ding", authorization: "not_determined", alerts: "disabled", canOpenSettings: true } } }));
  await page.route("**/v1/desktop/test", route => route.fulfill({ status: 503, json: { apiVersion: "ding.ing/v1alpha1", error: { code: "notification_helper_failed", message: "helper failed" } } }));
  let opened = false;
  await page.route("**/v1/desktop/settings", route => { opened = true; return route.fulfill({ json: { apiVersion: "ding.ing/v1alpha1", data: { opened: true } } }); });
  await page.getByRole("button", { name: "Set up notifications" }).click();
  const dialog = page.getByRole("dialog", { name: "Let Ding reach you" });
  await expect(dialog.getByText("Ding missing from the list?", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Send test notification", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("application error");
  await dialog.getByRole("button", { name: "Open notification settings" }).click();
  expect(opened).toBe(true);
  const a11y = await new AxeBuilder({ page }).analyze();
  expect(a11y.violations.filter(v => ["serious", "critical"].includes(v.impact ?? ""))).toEqual([]);
});

test("UI demo runs the real watch engine and keeps user watches empty", async ({ page, daemon }) => {
  test.setTimeout(90000);
  await page.goto(await daemon.launch());
  await page.getByRole("button", { name: "Try a one-minute demo" }).click();
  await page.getByRole("button", { name: "Start demo", exact: true }).click();
  await expect(page.getByText("Alert fired", { exact: true })).toBeVisible({ timeout: 50000 });
  await expect(page.getByText("Recovery recorded", { exact: true })).toBeVisible({ timeout: 30000 });
  await expect(page.getByText("Ding recorded a firing and a recovery.", { exact: false })).toBeVisible({ timeout: 25000 });
  const response = await fetch(daemon.url + "/v1/watches", { headers: { Authorization: "Bearer " + daemon.token } });
  expect((await response.json()).data).toHaveLength(0);
  const a11y = await new AxeBuilder({ page }).analyze();
  expect(a11y.violations.filter(v => ["serious", "critical"].includes(v.impact ?? ""))).toEqual([]);
});
