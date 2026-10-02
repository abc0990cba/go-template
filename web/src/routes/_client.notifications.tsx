import { createFileRoute } from "@tanstack/react-router";
import { NotificationsPage } from "@/features/notifications";

export const Route = createFileRoute("/_client/notifications")({
  staticData: { title: "Notifications" },
  component: NotificationsPage,
});
