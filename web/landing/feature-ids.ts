/**
 * The feature areas advertised on the landing, in display order. The ids
 * key the per-locale copy in locales/*.ts; the icons live in the FEATURES
 * map inside features.tsx. Adding a card means: extend this list, add the
 * copy to all four locales (the dictionary type makes the compiler demand
 * it), and attach an icon in features.tsx.
 */
export const FEATURE_IDS = [
  "auth",
  "rbac",
  "orgs",
  "tickets",
  "audit",
  "settings",
  "jobs",
  "streams",
  "webhooks",
  "notifications",
  "apikeys",
  "files",
  "export",
  "search",
  "resilience",
  "observability",
  "deploy",
  "dx",
] as const;

export type FeatureId = (typeof FEATURE_IDS)[number];
