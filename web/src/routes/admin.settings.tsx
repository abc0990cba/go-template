import { createFileRoute } from "@tanstack/react-router";
import { AdminSettingsPage } from "@/features/admin-settings";

export const Route = createFileRoute("/admin/settings")({
  staticData: { title: "Settings" },
  component: AdminSettingsPage,
});
