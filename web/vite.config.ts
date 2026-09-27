import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { defineConfig } from "vitest/config";

const apiTarget = "http://localhost:8080";

// The backend rejects state-changing requests whose Origin header mismatches
// the request host (internal/httpapp/security.go). The dev proxy otherwise
// forwards the browser's Origin (localhost:5173), so rewrite it to the API
// origin — the same-origin view the SPA has in the embedded production build.
const proxyOptions = {
  target: apiTarget,
  changeOrigin: true,
  headers: { Origin: apiTarget },
};

export default defineConfig({
  plugins: [
    // The router plugin must run before the React plugin so the generated
    // route tree exists by the time it is imported.
    TanStackRouterVite({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // Same-origin API access in dev: no CORS, cookies just work.
      "/api": proxyOptions,
      "/health": proxyOptions,
      "/docs": proxyOptions,
      "/openapi.yaml": proxyOptions,
      "/version": proxyOptions,
    },
  },
  build: {
    outDir: "dist",
    // Keep the committed .gitkeep so the Go binary always has an embeddable
    // directory even before the first frontend build.
    emptyOutDir: false,
    sourcemap: false,
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
  },
});
