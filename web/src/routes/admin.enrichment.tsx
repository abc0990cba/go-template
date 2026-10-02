import { createFileRoute } from "@tanstack/react-router";
import { listSearchSchema } from "@/shared/lib/search";
import { AdminEnrichmentPage } from "@/features/admin-ops";

export const Route = createFileRoute("/admin/enrichment")({
  validateSearch: listSearchSchema,
  staticData: { title: "Enrichment" },
  component: () => {
    const search = Route.useSearch();
    return <AdminEnrichmentPage search={search} />;
  },
});
