import { createFileRoute } from "@tanstack/react-router";
import { listSearchSchema } from "@/shared/lib/search";
import { AdminReportsPage } from "@/features/admin-insights";

export const Route = createFileRoute("/admin/reports")({
  validateSearch: listSearchSchema,
  staticData: { title: "Reports" },
  component: () => {
    const search = Route.useSearch();
    return <AdminReportsPage search={search} />;
  },
});
