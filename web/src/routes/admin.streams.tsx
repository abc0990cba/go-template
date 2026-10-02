import { createFileRoute } from "@tanstack/react-router";
import { AdminStreamsPage } from "@/features/admin-ops";

export const Route = createFileRoute("/admin/streams")({
  staticData: { title: "Streams" },
  component: AdminStreamsPage,
});
