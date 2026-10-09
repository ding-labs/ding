import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
  plugins: [react()],
  base: "/ui/",
  build: {
    outDir: "../../internal/webui/dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: { "/v1": { target: "http://127.0.0.1:7676", changeOrigin: false } },
  },
});
