import { build } from "esbuild";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const result = await build({
  absWorkingDir: root,
  entryPoints: ["src/main.tsx"],
  bundle: true,
  write: false,
  minify: true,
  format: "iife",
  platform: "browser",
  target: ["es2022"],
  define: { "process.env.NODE_ENV": '"production"' },
  outfile: "workspace.js",
});
const js = result.outputFiles.find((f) => f.path.endsWith(".js")).text;
const css = result.outputFiles.find((f) => f.path.endsWith(".css"))?.text ?? "";
const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Ding</title><style>${css.replaceAll("</style", "<\\/style")}</style></head><body><div id="root"></div><script>${js.replaceAll("</script", "<\\/script")}</script></body></html>`;
// The current official MCP Apps SDK includes protocol and schema validation.
// Keep the complete offline app (including React and that SDK) below 1 MiB.
if (Buffer.byteLength(html) > 1_048_576)
  throw new Error("MCP app exceeds its 1 MiB uncompressed budget");
const output = new URL(
  "../../../internal/mcpui/dist/",
  import.meta.url,
);
await mkdir(output, { recursive: true });
await writeFile(new URL("workspace.html", output), html);
console.log(
  `Bundled offline MCP app: ${(Buffer.byteLength(html) / 1024).toFixed(1)} KiB`,
);
