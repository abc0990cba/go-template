import createClient from "openapi-fetch";
import type { Paged, ListParams } from "@/shared/api/pagination";
import type { paths } from "./schema.gen";

/**
 * Transport core: one typed client for the whole SPA, with the wire shapes
 * generated from api/openapi/openapi.yaml (schema.gen.ts). Problem+json
 * errors become ApiError; the active org (Casbin domain) rides every call
 * as X-Org-Id. Feature modules own endpoints; this file only knows transport.
 */

/**
 * ApiError carries the problem+json details of a failed API call.
 * Network failures surface as ApiError with status 0.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly title: string;
  readonly detail: string;

  constructor(status: number, title: string, detail: string) {
    super(detail || title);
    this.name = "ApiError";
    this.status = status;
    this.title = title;
    this.detail = detail;
  }
}

interface ProblemBody {
  title?: string;
  detail?: string;
}

const ORG_HEADER_KEY = "app.activeOrg";

/**
 * The active organization id (Casbin domain) sent as X-Org-Id with every
 * request; empty string means the platform scope. Persisted so a reload
 * keeps the admin inside the tenant they switched to.
 */
export function getActiveOrg(): string {
  try {
    return localStorage.getItem(ORG_HEADER_KEY) ?? "";
  } catch {
    return "";
  }
}

export function setActiveOrg(orgId: string): void {
  try {
    if (orgId) localStorage.setItem(ORG_HEADER_KEY, orgId);
    else localStorage.removeItem(ORG_HEADER_KEY);
  } catch {
    // Private mode: the header just lives for this page session.
  }
}

/**
 * The fetch openapi-fetch invokes: adds the session cookie and the active
 * org header the same-origin backend expects, and maps network failures to
 * ApiError(status 0) before the client ever sees them.
 */
const transportFetch: typeof fetch = async (input, init) => {
  const orgId = getActiveOrg();
  const headers = new Headers(init?.headers);
  if (orgId) headers.set("X-Org-Id", orgId);
  try {
    return await fetch(input, { ...init, credentials: "include", headers });
  } catch (cause) {
    throw new ApiError(
      0,
      "network error",
      cause instanceof Error ? cause.message : "network error",
    );
  }
};

/**
 * The typed API client: paths, parameters, request bodies, and response
 * shapes all come from schema.gen.ts, so a spec change fails compilation at
 * the calling slice instead of at runtime. The base URL is the page origin —
 * openapi-fetch builds Request objects itself, and Node's Request (tests,
 * prerender) rejects relative URLs where browsers allow them — so every
 * request stays same-origin in the browser, through the dev proxy, and in
 * production behind the embedded-SPA router.
 */
export const api = createClient<paths>({
  baseUrl: typeof window === "undefined" ? undefined : window.location.origin,
  fetch: transportFetch,
});

/**
 * unwrap converts the client's {data, error} result into the app's throwing
 * style: 2xx resolves with the body (204 → undefined), and any error
 * response throws ApiError built from the problem+json fields — non-JSON
 * error bodies degrade to the status title, as before.
 */
export async function unwrap<T>(
  call: Promise<{
    data?: T;
    error?: unknown;
    response: Response;
  }>,
): Promise<T> {
  const { data, error, response } = await call;
  if (response.ok) return data as T;
  const problem = (error ?? {}) as ProblemBody;
  throw new ApiError(response.status, problem.title ?? "request failed", problem.detail ?? "");
}

export type { Paged, ListParams };
