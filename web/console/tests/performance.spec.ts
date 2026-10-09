import { test, expect } from "./fixtures";
import { call } from "./data";
test("measure first usable list with 1000 watches and warm assets", async ({
  page,
  daemon,
}) => {
  test.skip(
    process.env.DING_CONSOLE_PERF !== "1",
    "Opt-in reference-hardware measurement",
  );
  const definitions = Array.from(
    { length: 1000 },
    (_, i) =>
      `apiVersion: ding.ing/v1alpha1\nkind: Watch\nmetadata: {id: scale-${String(i).padStart(4, "0")}}\nspec:\n  source: {type: push}\n  condition: {field: status, operator: gte, value: 500}\n`,
  );
  for (let offset = 0; offset < definitions.length; offset += 100)
    await call(daemon, "/apply", {
      manifest: definitions.slice(offset, offset + 100).join("---\n"),
    });
  await page.goto(await daemon.launch());
  await expect(
    page.getByRole("link", { name: "scale-0000", exact: true }),
  ).toBeVisible();
  const measurements: number[] = [];
  for (let n = 0; n < 5; n++) {
    await page.goto("about:blank");
    const start = performance.now();
    await page.goto(daemon.url + "/ui/watches");
    await expect(
      page.getByRole("link", { name: "scale-0000", exact: true }),
    ).toBeVisible();
    measurements.push(Math.round(performance.now() - start));
  }
  console.log(
    "First usable 1000-watch list, warm assets, milliseconds:",
    measurements,
  );
  expect(Math.max(...measurements)).toBeLessThan(1000);
});
