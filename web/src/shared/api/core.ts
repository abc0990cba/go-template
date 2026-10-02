import type { Paged, ListParams } from "@/shared/api/pagination";

/**
 * Transport core: one fetch wrapper for the whole SPA. Problem+json errors
 * become ApiError; the active org (Casbin domain) rides every call as
 * X-Org-Id. Feature modules own endpoints; this file only knows transport.
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const orgId = getActiveOrg();
  let response: Response;
  try {
    response = await fetch(path, {
      credentials: "include",
      ...init,
      headers: {
        ...(init?.body === undefined ? {} : { "Content-Type": "application/json" }),
        ...(orgId ? { "X-Org-Id": orgId } : {}),
        ...init?.headers,
      },
    } satisfies RequestInit);
  } catch (cause) {
    throw new ApiError(
      0,
      "network error",
      cause instanceof Error ? cause.message : "network error",
    );
  }
  if (!response.ok) {
    let problem: ProblemBody = {};
    try {
      problem = (await response.json()) satisfies ProblemBody;
    } catch {
      // Non-JSON error bodies are fine; the status still carries meaning.
    }
    throw new ApiError(response.status, problem.title ?? "request failed", problem.detail ?? "");
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

/** Verbs the feature endpoint modules are built from. */
export const http = {
  get: <T>(path: string): Promise<T> => request<T>(path),
  post: <T>(path: string, body?: unknown): Promise<T> =>
    request<T>(path, {
      method: "POST",
      body: body === undefined ? undefined : JSON.stringify(body),
    }),
  put: <T>(path: string, body: unknown): Promise<T> =>
    request<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  patch: <T>(path: string, body: unknown): Promise<T> =>
    request<T>(path, { method: "PATCH", body: JSON.stringify(body) }),
  del: <T>(path: string, body?: unknown): Promise<T> =>
    request<T>(path, {
      method: "DELETE",
      body: body === undefined ? undefined : JSON.stringify(body),
    }),
};

export type { Paged, ListParams };
