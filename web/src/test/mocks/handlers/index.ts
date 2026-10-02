import { auth } from "./auth";
import { core } from "./core";
import { tickets } from "./tickets";
import { admin } from "./admin";

/** The combined handler list, one module per slice. Order matters only
 * within a module (first match wins); MSW picks the last matching handler
 * appended by server.use() overrides at runtime. */
export * from "../fixtures";

export const handlers = [...core, ...auth, ...tickets, ...admin];
