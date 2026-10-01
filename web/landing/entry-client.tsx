import { hydrateRoot } from "react-dom/client";
import { LandingPage } from "./LandingPage";
import "./landing.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("root element is missing");
}

// The markup was rendered by react-dom/server at build time
// (scripts/prerender.mjs); hydration attaches the event listeners without
// re-rendering, so the page is interactive even before this chunk loads.
hydrateRoot(root, <LandingPage />);
