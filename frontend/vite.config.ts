import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { UI_VERSION } from "./src/version";

const apiPort = process.env.MAGICHANDY_PORT || "49717";

export default defineConfig({
  plugins: [
    react(),
    {
      name: "magichandy-ui-version",
      transformIndexHtml(html) {
        return html.replace(
          "<head>",
          `<head>\n    <meta name="magichandy-ui-version" content="${UI_VERSION}" />`,
        );
      },
    },
  ],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/vitest.setup.ts"],
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: `http://127.0.0.1:${apiPort}`,
        changeOrigin: true,
      },
      "/healthz": {
        target: `http://127.0.0.1:${apiPort}`,
        changeOrigin: true,
      },
    },
  },
});
