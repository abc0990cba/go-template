import { api, unwrap } from "@/shared/api/core";
import type { SettingView } from "./types";

/** Endpoints of the admin-settings slice. Every call goes through the
 * typed client, so paths, bodies, and responses are checked against the
 * OpenAPI contract at compile time. */

export function settings(): Promise<SettingView[]> {
  return unwrap(api.GET("/api/settings")).then((p) => p.data);
}

export function updateSetting(key: string, value: string): Promise<void> {
  return unwrap(api.PUT("/api/settings/{key}", { params: { path: { key } }, body: { value } }));
}

export function publicSettings(): Promise<Record<string, string>> {
  return unwrap(api.GET("/api/settings/public"));
}
