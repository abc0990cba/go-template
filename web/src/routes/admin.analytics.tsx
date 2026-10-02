import { createFileRoute } from "@tanstack/react-router";
import { AdminAnalyticsPage } from "@/features/admin-insights";

export const Route = createFileRoute("/admin/analytics")({
  staticData: { title: "Analytics" },
  component: AdminAnalyticsPage,
});
