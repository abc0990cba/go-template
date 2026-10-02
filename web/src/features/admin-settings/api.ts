import { http } from "@/shared/api/core";
import type { Paged } from "@/shared/api/pagination";

/** Endpoints of the admin-settings slice. */
export function settings() {
  return http.get<Paged<SettingView>>("/api/settings").then((p) => p.data);
}

export function updateSetting(key: string, value: string) {
  return http.put<void>(`/api/settings/${encodeURIComponent(key)}`, { value });
}

export function publicSettings(): Promise<Record<string, string>> {
  return http.get("/api/settings/public");
}

import type { SettingView } from "./types";
