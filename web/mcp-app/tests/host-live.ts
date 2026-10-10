import {
  AppBridge,
  PostMessageTransport,
} from "@modelcontextprotocol/ext-apps/app-bridge";

declare global {
  interface Window {
    dingCall: (args: unknown) => Promise<any>;
    dingInitial: () => Promise<any>;
  }
}
const frame = document.querySelector("iframe")!;
const bridge = new AppBridge(
  null,
  { name: "Ding Go qualification", version: "1.0.0" },
  { serverTools: {} },
  { hostContext: { theme: "light", displayMode: "inline" } },
);
bridge.oncalltool = (args) => window.dingCall(args);
bridge.oninitialized = async () => {
  await bridge.sendToolInput({ arguments: {} });
  await bridge.sendToolResult(await window.dingInitial());
};
await bridge.connect(
  new PostMessageTransport(frame.contentWindow!, frame.contentWindow!),
);
