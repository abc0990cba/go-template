import { createFileRoute } from "@tanstack/react-router";
import { AdminWebhooksPage } from "@/features/admin-ops";

export const Route = createFileRoute("/admin/webhooks")({
  staticData: { title: "Webhooks" },
  component: AdminWebhooksPage,
});
