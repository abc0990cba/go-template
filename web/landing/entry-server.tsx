import { renderToString } from "react-dom/server";
import { LandingPage } from "./LandingPage";

/**
 * The SSR entry scripts/prerender.mjs imports after the Vite SSR build.
 * Kept separate from entry-client so the browser bundle never pulls in
 * react-dom/server.
 */
export function render(): string {
  return renderToString(<LandingPage />);
}
