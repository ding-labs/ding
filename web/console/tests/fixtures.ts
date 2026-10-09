import { test as base, expect } from "@playwright/test";
import { spawn, execFileSync, type ChildProcess } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, existsSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
const root = resolve(import.meta.dirname, "../../..");
export type Daemon = {
  url: string;
  state: string;
  token: string;
  ingest: string;
  launch: () => Promise<string>;
};
export const test = base.extend<{ daemon: Daemon }, { binary: string }>({
  binary: [
    async ({}, use) => {
      const dir = mkdtempSync(join(tmpdir(), "ding-console-bin-"));
      const bin = join(dir, process.platform === "win32" ? "ding.exe" : "ding");
      execFileSync(
        "go",
        ["build", "-tags", "console", "-o", bin, "./cmd/ding"],
        { cwd: root },
      );
      try {
        await use(bin);
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    },
    { scope: "worker" },
  ],
  daemon: async ({ binary }, use) => {
    const state = mkdtempSync(join(tmpdir(), "ding-console-test-"));
    const hook = createServer((req, res) => {
      req.resume();
      res.writeHead(400);
      res.end("test destination rejected request");
    });
    await new Promise<void>((resolve) => hook.listen(0, "127.0.0.1", resolve));
    const hookPort = (hook.address() as { port: number }).port;
    const child: ChildProcess = spawn(
      binary,
      ["daemon", "--listen", "127.0.0.1:0", "--state-dir", state],
      {
        stdio: "ignore",
        env: {
          ...process.env,
          DING_TEST_WEBHOOK_URL: `http://127.0.0.1:${hookPort}`,
        },
      },
    );
    try {
      await expect
        .poll(() => existsSync(join(state, "connection.json")), {
          timeout: 10000,
        })
        .toBeTruthy();
      const { url } = JSON.parse(
        readFileSync(join(state, "connection.json"), "utf8"),
      );
      const { admin, ingest } = JSON.parse(
        readFileSync(join(state, "tokens.json"), "utf8"),
      );
      const launch = async () => {
        const r = await fetch(url + "/v1/browser/handoff", {
          method: "POST",
          headers: {
            Authorization: "Bearer " + admin,
            "Content-Type": "application/json",
          },
          body: "{}",
        });
        const body = await r.json();
        if (!r.ok) throw new Error(body.error?.message);
        return body.data.url as string;
      };
      await use({ url, state, token: admin, ingest, launch });
    } finally {
      child.kill("SIGTERM");
      await new Promise<void>((resolve) => {
        if (child.exitCode !== null) resolve();
        else child.once("exit", () => resolve());
      });
      await new Promise<void>((resolve) => hook.close(() => resolve()));
      rmSync(state, { recursive: true, force: true });
    }
  },
});
export { expect };
