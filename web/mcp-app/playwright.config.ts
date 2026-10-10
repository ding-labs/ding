import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  use: { viewport: { width: 780, height: 920 }, headless: true },
  reporter: "list",
});
