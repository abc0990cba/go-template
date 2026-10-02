import { queryClient } from "@/shared/lib/query-client";
import { queryOptions } from "@tanstack/react-query";
import { redirect } from "@tanstack/react-router";
import { http } from "@/shared/api/core";
import type { Me } from "@/shared/types/core";

/**
 * Session query: the SPA's single source of truth for "who is logged in".
 * The cookie is the credential; the query is the cache around GET /api/me.
 */
export function meQuery() {
  return queryOptions({
    queryKey: ["me"],
    queryFn: () => http.get<Me>("/api/me"),
    staleTime: 60_000,
    retry: false,
  });
}

/**
 * ensureAuth is the beforeLoad guard of the authenticated layout: it
 * resolves the session (deduped by the query client) and redirects
 * anonymous visitors to the login page, remembering where they headed.
 */
export async function ensureAuth(location: { href: string }) {
  try {
    await queryClient.ensureQueryData(meQuery());
  } catch {
    throw redirect({
      to: "/login",
      search: { redirect: location.href },
    });
  }
}

/**
 * ensurePermission layers an RBAC check on top of ensureAuth: without the
 * expanded "object:action" permission the visitor is sent back to the
 * dashboard instead of meeting a bare 403 screen.
 */
export async function ensurePermission(object: string, action: string, location: { href: string }) {
  await ensureAuth(location);
  const me = queryClient.getQueryData<Me>(meQuery().queryKey);
  if (!me?.permissions.includes(`${object}:${action}`)) {
    // The landing page owns "/", so the fallback home is the dashboard.
    throw redirect({ to: "/dashboard" });
  }
}
