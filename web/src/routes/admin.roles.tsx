import { createFileRoute } from "@tanstack/react-router";
import { AdminRolesPage } from "@/features/admin-rbac";

export const Route = createFileRoute("/admin/roles")({
  staticData: { title: "Roles" },
  component: AdminRolesPage,
});
