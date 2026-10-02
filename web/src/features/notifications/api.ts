import { api, unwrap } from "@/shared/api/core";
import type { Notification } from "./types";

/** Endpoints of the notifications slice. Every call goes through the typed
 * client, so paths, bodies, and responses are checked against the OpenAPI
 * contract at compile time. */

export function notifications(limit = 25): Promise<Notification[]> {
  return unwrap(api.GET("/api/notifications", { params: { query: { limit } } })).then(
    (p) => p.data,
  );
}

export function unreadCount(): Promise<{ count: number }> {
  return unwrap(api.GET("/api/notifications/unread-count"));
}

export function markNotificationRead(id: string): Promise<void> {
  return unwrap(api.POST("/api/notifications/{id}/read", { params: { path: { id } } }));
}

export function markAllNotificationsRead(): Promise<void> {
  return unwrap(api.POST("/api/notifications/read-all"));
}
