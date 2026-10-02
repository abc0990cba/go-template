import { createFileRoute } from "@tanstack/react-router";
import { AdminOrgsPage } from "@/features/admin-orgs";

export const Route = createFileRoute("/admin/orgs")({
  staticData: { title: "Organizations" },
  component: AdminOrgsPage,
});
