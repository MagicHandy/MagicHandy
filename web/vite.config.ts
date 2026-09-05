/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import {fileURLToPath} from "node:url";

// Static build only. Output goes to web/dist and is embedded by web/assets.go;
// the runtime is the Go binary, never a Node/Vite server. base "/" because the
// Go server serves the app from the domain root and /assets/* from the same FS.
export default defineConfig({
  plugins: [{
    name:"runtime-labs-build",
    transformIndexHtml() { return [{tag:"meta",attrs:{name:"magichandy-build",content:"release-with-optional-labs"},injectTo:"head" as const}]; },
  },react()],
  resolve: {alias:{"@labs":fileURLToPath(new URL("./src/labs/enabled.tsx",import.meta.url))}},
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // The optional Labs workspace and its CSS load only when opened.
    chunkSizeWarningLimit: 900,
  },
  server: { port: 5173 },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/vitest.setup.ts"],
    css: false,
  },
});
