import { createFileRoute } from "@tanstack/react-router";
import { AdminGettingStartedPage } from "@/features/admin-settings";

export const Route = createFileRoute("/admin/getting-started")({
  staticData: { title: "Getting Started" },
  component: AdminGettingStartedPage,
});
