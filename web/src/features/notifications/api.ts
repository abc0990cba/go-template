import { http } from "@/shared/api/core";
import type { Paged } from "@/shared/api/pagination";
import type { Notification } from "./types";

/** Endpoints of the notifications slice. */
export function notifications(limit = 25): Promise<Notification[]> {
  return http.get<Paged<Notification>>(`/api/notifications?limit=${limit}`).then((p) => p.data);
}

export function unreadCount(): Promise<{ count: number }> {
  return http.get("/api/notifications/unread-count");
}

export function markNotificationRead(id: string): Promise<void> {
  return http.post<void>(`/api/notifications/${encodeURIComponent(id)}/read`);
}

export function markAllNotificationsRead(): Promise<void> {
  return http.post<void>("/api/notifications/read-all");
}
