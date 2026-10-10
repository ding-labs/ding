// Protocol-faithful host fixture: the production widget connects through the
// actual SDK bridge and postMessage transport, not a test-only UI data hook.
import {
  AppBridge,
  PostMessageTransport,
} from "@modelcontextprotocol/ext-apps/app-bridge";

const rows = [
  {
    id: "payments",
    name: "Payments API",
    status: "running",
    source: "http",
    open: 0,
    unhealthy: 0,
    failed: 0,
    pending: 0,
    revision: "a".repeat(64),
    generation: 1,
  },
  {
    id: "worker-heartbeat",
    name: "Worker heartbeat",
    status: "running",
    source: "push",
    open: 1,
    unhealthy: 0,
    failed: 0,
    pending: 0,
    revision: "b".repeat(64),
    generation: 1,
  },
  {
    id: "inventory",
    name: "Inventory changes",
    status: "paused",
    source: "http",
    open: 0,
    unhealthy: 0,
    failed: 0,
    pending: 0,
    revision: "c".repeat(64),
    generation: 2,
  },
];
const wrap = (view: string, data: Record<string, unknown>) => ({
  content: [],
  structuredContent: { view, data },
});
const initial = wrap("watches", {
  watches: rows,
  total: 3,
  all: 3,
  attention: 1,
  paused: 1,
  cursor: "",
  more: false,
});
const iframe = document.querySelector("iframe")!;
const bridge = new AppBridge(
  null,
  { name: "Ding qualification host", version: "1.0.0" },
  { serverTools: {} },
  {
    hostContext: {
      theme:
        new URLSearchParams(location.search).get("theme") === "dark"
          ? "dark"
          : "light",
      displayMode: "inline",
    },
  },
);
bridge.oncalltool = async ({ name }) =>
  name === "ding_get_capabilities"
    ? wrap("capabilities", {
        grant: { scopes: ["inspect", "preview", "manage"] },
      })
    : name === "ding_list_events"
      ? wrap("events", { events: [], more: false, cursor: "", total: 0 })
      : initial;
bridge.oninitialized = async () => {
  await bridge.sendToolInput({ arguments: {} });
  await bridge.sendToolResult(initial);
};
await bridge.connect(
  new PostMessageTransport(iframe.contentWindow!, iframe.contentWindow!),
);
