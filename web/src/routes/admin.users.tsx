import { createFileRoute } from "@tanstack/react-router";
import { listSearchSchema } from "@/shared/lib/search";
import { AdminUsersPage } from "@/features/admin-users";

export const Route = createFileRoute("/admin/users")({
  validateSearch: listSearchSchema,
  staticData: { title: "Users" },
  component: () => {
    const search = Route.useSearch();
    return <AdminUsersPage search={search} />;
  },
});
