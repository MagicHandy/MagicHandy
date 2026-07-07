/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Static build only. Output goes to web/dist and is embedded by web/assets.go;
// the runtime is the Go binary, never a Node/Vite server. base "/" because the
// Go server serves the app from the domain root and /assets/* from the same FS.
export default defineConfig({
  plugins: [react()],
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // No hashed sub-chunks proliferation; keep a lean single app bundle.
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
