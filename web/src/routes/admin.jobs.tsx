import { createFileRoute } from "@tanstack/react-router";
import { listSearchSchema } from "@/shared/lib/search";
import { AdminJobsPage } from "@/features/admin-ops";

export const Route = createFileRoute("/admin/jobs")({
  validateSearch: listSearchSchema,
  staticData: { title: "Jobs" },
  component: () => {
    const search = Route.useSearch();
    return <AdminJobsPage search={search} />;
  },
});
