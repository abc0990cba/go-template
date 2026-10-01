import { setupServer } from "msw/node";
import { handlers } from "./handlers";

/**
 * The request-level mock of the backend for jsdom tests. Imported by the
 * Vitest setup file, which owns the listen/reset/close lifecycle.
 */
export const server = setupServer(...handlers);
