import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { resolve, join } from "node:path";
import { randomUUID } from "node:crypto";
import { runtime, manifest } from "./runtime.mjs";

const r = await runtime();
try {
  const tools = await r.client.listTools();
  assert.equal(tools.tools.length, 15);
  const call = async (name, args = {}, client = r.client) => {
    const result = await client.callTool({ name, arguments: args });
    assert.ok(!result.isError, JSON.stringify(result));
    assert.deepEqual(
      JSON.parse(result.content[0].text),
      result.structuredContent,
    );
    return result.structuredContent.data;
  };
  for (const name of [
    "ding_get_capabilities",
    "ding_list_watches",
    "ding_list_events",
    "ding_list_deliveries",
    "ding_list_destinations",
  ])
    await call(name);
  const page = await r.client.readResource({ uri: "ui://ding/workspace.html" });
  assert.equal(
    page.contents[0].text,
    await readFile(
      new URL("../../../internal/mcpui/dist/workspace.html", import.meta.url),
      "utf8",
    ),
  );
  const preview = await call("ding_preview_changes", { manifest });
  assert.equal(preview.valid, true);
  const operation_key = randomUUID();
  const args = { handle: preview.preview.handle, operation_key };
  assert.deepEqual(
    await call("ding_apply_changes", args),
    await call("ding_apply_changes", args),
  );
  const watch = (await call("ding_get_watch", { watch_id: "heartbeat" })).watch;
  assert.equal(
    (
      await call("ding_pause_watch", {
        watch_id: "heartbeat",
        expected_revision: watch.plan.revision,
        expected_generation: watch.generation,
        operation_key: randomUUID(),
      })
    ).status,
    "paused",
  );
  const events = await call("ding_list_events", { watch: "heartbeat" });
  await call("ding_get_event", { event_id: events.events[0].id });
  assert.equal(
    (await call("ding_get_operation", { operation_key })).action,
    "apply",
  );
  // The main binary mounts the same commands and accepts the exact same config.
  const main = await r.connect(resolve(process.env.DING_TEST_BINARY), ["mcp"]);
  assert.equal((await call("ding_list_watches", {}, main)).watches.length, 1);
  await main.close();
  const grant = JSON.parse(await readFile(r.config, "utf8"));
  const admin = JSON.parse(
    await readFile(join(r.state, "tokens.json"), "utf8"),
  ).admin;
  const revoked = await fetch(
    `${r.connection.url}/v1/integrations/grants/${grant.grant_id}`,
    { method: "DELETE", headers: { Authorization: `Bearer ${admin}` } },
  );
  assert.equal(revoked.status, 200);
  const denied = await r.client.callTool({
    name: "ding_apply_changes",
    arguments: args,
  });
  assert.equal(denied.isError, true);
  assert.match(denied.content[0].text, /integration_denied/);
  // Adapter shutdown did not stop the independently running daemon.
  await r.client.close();
  assert.equal(
    (
      await fetch(`${r.connection.url}/v1/integrations/grants`, {
        headers: { Authorization: `Bearer ${admin}` },
      })
    ).status,
    200,
  );
  console.log(
    "Independent TypeScript SDK: native stdio, 15 tools, UI bytes, pairing, both CLI entry points, review/apply, receipt retry, lifecycle, evidence, revocation, daemon independence passed.",
  );
} finally {
  await r.close();
}
