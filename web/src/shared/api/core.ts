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
  // openapi-fetch hands fetch a fully-built Request (Content-Type, body and
  // friends already set). Any member present in init REPLACES the Request's
  // counterpart, so passing a bare `headers` object wiped them — sign-in
  // went out with an empty Content-Type and the runtime contract gate
  // rejected it. Start from the Request's own headers and layer init's on
  // top, then add the tenant header last.
  const headers = new Headers(init?.headers ?? undefined);
  if (input instanceof Request) {
    for (const [key, value] of input.headers) {
      if (!headers.has(key)) headers.set(key, value);
    }
  }
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

/**
 * unwrapWithETag is unwrap for conditional writes: it also returns the
 * response's ETag (empty when absent), which the caller echoes back as
 * If-Match so a concurrent edit fails with 412 instead of clobbering.
 */
export async function unwrapWithETag<T>(
  call: Promise<{
    data?: T;
    error?: unknown;
    response: Response;
  }>,
): Promise<{ data: T; etag: string }> {
  const { data, error, response } = await call;
  if (response.ok) {
    return { data: data as T, etag: response.headers.get("ETag") ?? "" };
  }
  const problem = (error ?? {}) as ProblemBody;
  throw new ApiError(response.status, problem.title ?? "request failed", problem.detail ?? "");
}

/**
 * presignedPut uploads raw bytes to a presigned storage URL (S3/MinIO part
 * uploads) and returns the response's ETag. It lives beside the fetch
 * wrapper: the URL is storage-signed, not an API endpoint, so it cannot go
 * through the typed client — but the transport rules (credentials, error
 * mapping) stay in this one place.
 */
export async function presignedPut(url: string, body: Blob): Promise<string> {
  const response = await fetch(url, { method: "PUT", body });
  if (!response.ok) {
    throw new ApiError(response.status, "part upload failed", "");
  }
  return response.headers.get("ETag") ?? "";
}

/**
 * isStaleWrite reports an optimistic-locking rejection (HTTP 412): the
 * resource changed since the caller last read it. The UI reloads the
 * current state and asks the user to re-apply their edit.
 */
export function isStaleWrite(error: unknown): boolean {
  return error instanceof ApiError && error.status === 412;
}

export type { Paged, ListParams };
