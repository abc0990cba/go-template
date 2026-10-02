import { createFileRoute } from "@tanstack/react-router";
import { listSearchSchema } from "@/shared/lib/search";
import { AdminAuditPage } from "@/features/admin-insights";

export const Route = createFileRoute("/admin/audit")({
  validateSearch: listSearchSchema,
  staticData: { title: "Audit Log" },
  component: () => {
    const search = Route.useSearch();
    return <AdminAuditPage search={search} />;
  },
});
