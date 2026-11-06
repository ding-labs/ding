import { test, expect } from "./fixtures";
import AxeBuilder from "@axe-core/playwright";

test("onboarding reuses a saved destination and reconciles a lost test response", async ({ page, daemon }) => {
  const manifest = "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: saved-console}\nspec: {type: console}\n";
  const seed = await fetch(daemon.url + "/v1/apply", { method: "POST", headers: { Authorization: "Bearer " + daemon.token, "Content-Type": "application/json" }, body: JSON.stringify({ manifest }) });
  expect(seed.ok).toBeTruthy();
  const readDestinations = async () => {
    const r = await fetch(daemon.url + "/v1/console/destinations", { headers: { Authorization: "Bearer " + daemon.token } });
    return (await r.json()).data.destinations;
  };
  const before = await readDestinations();
  const keys: string[] = [];
  await page.route("**/v1/onboarding/destination-test", async route => {
    keys.push(route.request().postDataJSON().operationKey);
    const response = await route.fetch();
    if (keys.length === 1) await route.abort();
    else await route.fulfill({ response });
  });
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Create your first watch" }).click();
  await page.getByLabel("Health endpoint").fill(daemon.webhookURL);
  await page.getByLabel("Saved destination").selectOption("saved-console");
  await page.getByRole("button", { name: "Preview watch", exact: true }).click();
  await expect(page.getByRole("button", { name: "Start this watch" })).toBeDisabled();
  await page.getByRole("button", { name: "Send saved destination test" }).click();
  await page.getByRole("button", { name: "Check saved destination test outcome" }).click();
  await expect(page.getByRole("button", { name: "Start this watch" })).toBeDisabled();
  await page.getByLabel("I received the test at this destination").check();
  await page.getByRole("button", { name: "Start this watch" }).click();
  await expect(page).toHaveURL(/watches\/first-watch$/);
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
  const after = await readDestinations();
  expect(after).toHaveLength(1);
  expect(after[0].revision).toBe(before[0].revision);
  expect(after[0].definition).toEqual(before[0].definition);
  expect(after[0].watches).toBe(1);
});

test("first useful watch is reviewed and observed without an account", async ({ page, daemon }) => {
  await page.goto(await daemon.launch());
  await page.getByRole("link", { name: "Create your first watch" }).click();
  await expect(page.getByRole("heading", { name: "Your first useful watch" })).toBeVisible();
  await page.getByLabel("Watch ID").fill("local-health");
  await page.getByLabel("Health endpoint").fill(daemon.webhookURL);
  await page.getByLabel("Alert delivery").selectOption("console");
  await page.getByRole("button", { name: "Preview watch" }).click();
  await expect(page.getByRole("heading", { name: "Review before starting" })).toBeVisible();
  await expect(page.getByText(/every 30s \(timeout 5s\)/)).toBeVisible();
  await expect(page.getByText(/401 or 404 do not match/)).toBeVisible();
  await expect(page.getByText(/3 matching checks span 1m0s/)).toBeVisible();
  const before = await fetch(daemon.url + "/v1/watches", { headers: { Authorization: "Bearer " + daemon.token } });
  expect((await before.json()).data).toHaveLength(0);
  const a11y = await new AxeBuilder({ page }).analyze();
  expect(a11y.violations.filter(v => ["serious", "critical"].includes(v.impact ?? ""))).toEqual([]);
  await page.screenshot({ path: "/tmp/ding-first-watch.png", fullPage: true });
  let state: "waiting" | "source-error" | "delivery-error" | "live" = "waiting";
  await page.route("**/v1/console/watches?**", async route => {
    const response = await route.fetch();
    const body = await response.json();
    if (new URL(route.request().url()).searchParams.get("watch") === "local-health" && state !== "live") {
      for (const watch of body.data.watches) {
        if (state === "waiting") Object.assign(watch, { lastInputAt: "0001-01-01T00:00:00Z", entities: 0 });
        if (state === "source-error") Object.assign(watch, { lastError: "connection timed out", unhealthy: 1 });
        if (state === "delivery-error") Object.assign(watch, { failed: 1, pending: 1 });
      }
    }
    await route.fulfill({ response, json: body });
  });
  await page.getByRole("button", { name: "Start this watch" }).click();
  await expect(page).toHaveURL(/watches\/local-health$/);
  await expect(page.locator("#first-observation-title")).toHaveText("Waiting for the first observation");
  await expect(page.getByRole("link", { name: "Create another watch" })).toHaveCount(0);
  state = "source-error";
  await expect(page.locator("#first-observation-title")).toHaveText("Monitoring needs attention", { timeout: 10000 });
  await expect(page.getByRole("link", { name: "Create another watch" })).toHaveCount(0);
  state = "delivery-error";
  await expect(page.getByText(/A notification could not be delivered/)).toBeVisible({ timeout: 10000 });
  await expect(page.getByRole("link", { name: "Inspect deliveries" })).toBeVisible();
  state = "live";
  await expect(page.getByText(/A notification could not be delivered/)).toHaveCount(0, { timeout: 10000 });
  await expect.poll(async () => {
    const r = await fetch(daemon.url + "/v1/doctor", { headers: { Authorization: "Bearer " + daemon.token } });
    const source = (await r.json()).data.sources.find((s: { id: string }) => s.id === "local-health");
    return !!source && !source.lastInputAt.startsWith("0001");
  }).toBeTruthy();
  await expect(page.locator("#first-observation-title")).toHaveText("Monitoring input received");
  await expect(page.getByRole("link", { name: "Create another watch" })).toBeVisible();
  await expect(page.getByText(/not service health or notification delivery/)).toBeVisible();
  await page.getByRole("link", { name: "Create another watch" }).click();
  await expect(page.getByLabel("Watch ID")).toHaveValue("");
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

test("protected destination setup confirms receipt across lost responses and reloads", async ({ page, daemon }) => {
  const headers = { Authorization: "Bearer " + daemon.token, "Content-Type": "application/json" };
  const manifest = "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: chat-alerts}\nspec: {type: console}\n";
  expect((await fetch(daemon.url + "/v1/apply", { method: "POST", headers, body: JSON.stringify({ manifest }) })).ok).toBeTruthy();
  const status = async () => (await (await fetch(daemon.url + "/v1/onboarding/destination-status?destination=chat-alerts", { headers })).json()).data;
  const before = await status();
  let sends = 0, confirmations = 0;
  await page.route("**/v1/onboarding/destination-test", async route => {
    sends++;
    await route.fetch();
    await route.abort();
  });
  await page.route("**/v1/onboarding/destination-confirm", async route => {
    confirmations++;
    await route.fetch();
    await route.abort();
  });
  await page.goto(await daemon.launch());
  await expect(page.getByRole("link", { name: "Create your first watch" })).toBeVisible();
  await page.goto(daemon.url + before.setupURL);
  await page.getByRole("button", { name: "Send labeled notification test", exact: true }).click();
  await expect(page.getByRole("button", { name: "Refresh delivery status" })).toBeEnabled();
  await page.reload();
  await page.getByRole("button", { name: "I received the test at this destination" }).click();
  await expect(page.getByRole("button", { name: "Refresh delivery status" })).toBeEnabled();
  await page.getByRole("button", { name: "Refresh delivery status" }).click();
  await expect(page.getByRole("status", { name: "Notification readiness" })).toContainText("Receipt confirmed");
  await page.reload();
  await expect(page.getByRole("status", { name: "Notification readiness" })).toContainText("Receipt confirmed");
  expect(sends).toBe(1); expect(confirmations).toBe(1);
  expect((await status()).state).toBe("confirmed");
  expect((await (await fetch(daemon.url + "/v1/watches", { headers })).json()).data).toHaveLength(0);
  const a11y = await new AxeBuilder({ page }).analyze();
  expect(a11y.violations.filter(v => ["serious", "critical"].includes(v.impact ?? ""))).toEqual([]);
  await page.screenshot({ path: "/tmp/ding-destination-setup.png", fullPage: true });
});

test("destination-only setup creates delivery without a watch and blocks the wrong account", async ({ page, daemon }) => {
  await page.goto(await daemon.launch());
  await expect(page.getByRole("link", { name: "Create your first watch" })).toBeVisible();
  await page.goto(daemon.url + "/ui/destination-setup?account=another-account");
  await expect(page.getByRole("heading", { name: "This setup link belongs to another account or workspace" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Create notification destination" })).toHaveCount(0);
  await page.goto(daemon.url + "/ui/destination-setup");
  await page.getByLabel("Destination ID", { exact: true }).fill("new-chat-alerts");
  await page.getByLabel("Delivery type").selectOption("console");
  await page.getByRole("button", { name: "Create notification destination" }).click();
  await expect(page).toHaveURL(/destination=new-chat-alerts/);
  await page.getByRole("button", { name: "Send labeled notification test", exact: true }).click();
  await page.getByRole("button", { name: "I received the test at this destination" }).click();
  await expect(page.getByRole("status", { name: "Notification readiness" })).toContainText("Receipt confirmed");
  const headers = { Authorization: "Bearer " + daemon.token };
  expect((await (await fetch(daemon.url + "/v1/watches", { headers })).json()).data).toHaveLength(0);
  expect((await (await fetch(daemon.url + "/v1/console/destinations", { headers })).json()).data.destinations).toHaveLength(1);
});
