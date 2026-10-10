import { test, expect } from "@playwright/test";
import { build } from "esbuild";
import { runtime, manifest } from "./runtime.mjs";

test("embedded app reviews and applies through the native Go server", async ({
  page,
}) => {
  test.skip(
    !process.env.DING_TEST_BINARY || !process.env.DING_MCP_BINARY,
    "Native binaries are required for live qualification",
  );
  const r = await runtime();
  try {
    const preview = await r.client.callTool({
      name: "ding_preview_changes",
      arguments: { manifest },
    });
    const resource = await r.client.readResource({
      uri: "ui://ding/workspace.html",
    });
    const built = await build({
      entryPoints: ["tests/host-live.ts"],
      bundle: true,
      write: false,
      format: "esm",
      platform: "browser",
      target: "es2022",
    });
    await page.exposeFunction("dingCall", async (args) =>
      r.client.callTool(args),
    );
    await page.exposeFunction("dingInitial", () => preview);
    const external: string[] = [];
    await page.route("**/*", async (route) => {
      const u = new URL(route.request().url());
      if (u.origin !== "https://ding-test.local") {
        external.push(u.href);
        return route.abort();
      }
      if (u.pathname === "/widget")
        return route.fulfill({
          contentType: "text/html",
          headers: {
            "Content-Security-Policy":
              "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; base-uri 'none'",
          },
          body: resource.contents[0].text,
        });
      if (u.pathname === "/host.js")
        return route.fulfill({
          contentType: "text/javascript",
          body: built.outputFiles[0].text,
        });
      return route.fulfill({
        contentType: "text/html",
        body: '<!doctype html><iframe title="Ding" src="/widget" style="width:100%;height:850px;border:0"></iframe><script type="module" src="/host.js"></script>',
      });
    });
    await page.goto("https://ding-test.local");
    const frame = page.frameLocator("iframe");
    await expect(
      frame.getByRole("heading", { name: "Review your changes" }),
    ).toBeVisible();
    const before = await r.client.callTool({
      name: "ding_list_watches",
      arguments: {},
    });
    expect(before.structuredContent.data.watches).toHaveLength(0);
    await frame.getByRole("button", { name: "Apply reviewed changes" }).click();
    const after = await r.client.callTool({
      name: "ding_list_watches",
      arguments: {},
    });
    await expect
      .poll(
        async () =>
          (
            await r.client.callTool({
              name: "ding_list_watches",
              arguments: {},
            })
          ).structuredContent.data.watches.length,
      )
      .toBe(1);
    await frame.getByRole("button", { name: "Watches", exact: true }).click();
    await expect(frame.getByText("Heartbeat", { exact: true })).toBeVisible();
    expect(external).toEqual([]);
    await page.screenshot({
      path: "test-results/ding-mcp-go-live.png",
      fullPage: true,
    });
  } finally {
    await r.close();
  }
});
