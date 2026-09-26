import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { defineConfig } from "vitest/config";

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
      "/api": "http://localhost:8080",
      "/health": "http://localhost:8080",
      "/docs": "http://localhost:8080",
      "/openapi.yaml": "http://localhost:8080",
      "/version": "http://localhost:8080",
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
