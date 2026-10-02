import { createFileRoute } from "@tanstack/react-router";
import { AdminPoliciesPage } from "@/features/admin-rbac";

export const Route = createFileRoute("/admin/policies")({
  staticData: { title: "Policies" },
  component: AdminPoliciesPage,
});
