import { spawn, execFile } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { setTimeout as delay } from "node:timers/promises";
import { Client } from "@modelcontextprotocol/client";
import { StdioClientTransport } from "@modelcontextprotocol/client/stdio";

export const manifest = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: heartbeat, name: Heartbeat}
spec:
  source: {type: push}
  condition: {missingFor: 5m}
`;

// A real daemon and native adapter, controlled by an independent SDK. No fixture
// grants, developer accounts, or user state are touched by this harness.
export async function runtime() {
  if (!process.env.DING_TEST_BINARY || !process.env.DING_MCP_BINARY)
    throw new Error(
      "Build Ding and its Go MCP adapter; set DING_TEST_BINARY and DING_MCP_BINARY.",
    );
  const root = await mkdtemp(join(tmpdir(), "ding-mcp-interop-"));
  const state = join(root, "state");
  const config = join(root, "mcp.json");
  const daemon = spawn(
    resolve(process.env.DING_TEST_BINARY),
    ["daemon", "--state-dir", state, "--listen", "127.0.0.1:0"],
    { stdio: "ignore" },
  );
  const exited = new Promise((done) => {
    daemon.once("exit", done);
    daemon.once("error", done);
  });
  const clients = [];
  const close = async () => {
    for (const client of clients) await client.close().catch(() => {});
    if (daemon.exitCode === null) daemon.kill();
    await exited;
    await rm(root, { recursive: true, force: true });
  };
  try {
    let connection;
    for (let i = 0; i < 200; i++) {
      try {
        connection = JSON.parse(
          await readFile(join(state, "connection.json"), "utf8"),
        );
        break;
      } catch {
        if (daemon.exitCode !== null) throw new Error("Test daemon exited");
        await delay(25);
      }
    }
    if (!connection) throw new Error("Test daemon failed to start");
    const binary = resolve(process.env.DING_MCP_BINARY);
    const paired = await promisify(execFile)(binary, [
      "pair",
      "--state",
      state,
      "--config",
      config,
      "--manage",
      "--retry",
    ]);
    if (paired.stdout.includes("ding_mcp_"))
      throw new Error("Pairing printed a credential");
    const connect = async (command = binary, prefix = []) => {
      const client = new Client({
        name: "Ding independent qualification",
        version: "1.0.0",
      });
      clients.push(client);
      await client.connect(
        new StdioClientTransport({
          command,
          args: [...prefix, "serve", "--config", config],
          stderr: "pipe",
        }),
      );
      return client;
    };
    const client = await connect();
    return { client, connect, close, config, state, connection, binary };
  } catch (error) {
    await close();
    throw error;
  }
}
