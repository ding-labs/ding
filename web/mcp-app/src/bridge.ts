import { App } from "@modelcontextprotocol/ext-apps";

export type ToolResult = {
  isError?: boolean;
  structuredContent?: unknown;
  content?: unknown[];
};
export type DingView = {
  view: string;
  data: Record<string, unknown>;
  query?: Record<string, unknown>;
};
export interface Bridge {
  connect(onResult: (result: ToolResult) => void): Promise<void>;
  call(name: string, args: Record<string, unknown>): Promise<ToolResult>;
}

export function parseResult(result: ToolResult): DingView {
  if (result.isError) {
    const text = (result.content ?? [])
      .map((c) =>
        typeof c === "object" && c && "text" in c ? String(c.text) : "",
      )
      .join("\n");
    throw new Error(
      text.slice(0, 2000) || "Ding could not complete this request.",
    );
  }
  const data = result.structuredContent;
  if (
    !data ||
    typeof data !== "object" ||
    !("view" in data) ||
    !("data" in data) ||
    typeof data.view !== "string" ||
    !data.data ||
    typeof data.data !== "object"
  ) {
    throw new Error("This result needs a newer Ding integration.");
  }
  return data as DingView;
}

export function hostBridge(): Bridge {
  const app = new App(
    { name: "Ding", version: "0.1.0" },
    {},
    { autoResize: true },
  );
  return {
    async connect(onResult) {
      // Register before connect: hosts may send the initial result during the handshake.
      app.ontoolresult = onResult;
      app.onhostcontextchanged = (context) => {
        if (context.theme)
          document.documentElement.dataset.theme = context.theme;
      };
      await app.connect();
      const context = app.getHostContext();
      if (context?.theme)
        document.documentElement.dataset.theme = context.theme;
    },
    call: (name, args) => app.callServerTool({ name, arguments: args }),
  };
}
