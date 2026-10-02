import { createFileRoute } from "@tanstack/react-router";
import { AdminIndexPage } from "@/features/admin-insights";

export const Route = createFileRoute("/admin/")({
  staticData: { title: "Dashboard" },
  component: AdminIndexPage,
});
