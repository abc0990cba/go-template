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

/**
 * In production the Go binary serves the prerendered landing at "/" and the
 * SPA everywhere else (internal/httpapp/landing.go). The dev middleware
 * mirrors that split: "/" serves the landing entry, extension-less paths
 * keep falling back to the SPA's index.html.
 */
function landingAtRoot() {
  return {
    name: "landing-at-root",
    configureServer(server: {
      middlewares: {
        use: (cb: (req: { url?: string }, _res: unknown, next: () => void) => void) => void;
      };
    }) {
      server.middlewares.use((req, _res, next) => {
        if (req.url === "/") {
          req.url = "/landing/index.html";
        }
        next();
      });
    },
  };
}

export default defineConfig({
  plugins: [
    // The router plugin must run before the React plugin so the generated
    // route tree exists by the time it is imported.
    TanStackRouterVite({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
    landingAtRoot(),
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
    // Empty per build: with hashed filenames, keeping old output would
    // accumulate stale chunks in the go:embed payload forever. The committed
    // dist/.gitkeep placeholder is re-created by scripts/prerender.mjs.
    emptyOutDir: true,
    sourcemap: false,
    // Two entries: the admin SPA (index.html) and the landing
    // (landing/index.html). Each gets its own JS/CSS chunks, so landing
    // visitors never download the admin bundle.
    rollupOptions: {
      input: {
        main: path.resolve(import.meta.dirname, "index.html"),
        landing: path.resolve(import.meta.dirname, "landing/index.html"),
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
  },
});
