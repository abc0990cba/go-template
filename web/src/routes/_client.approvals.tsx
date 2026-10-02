import { createFileRoute } from "@tanstack/react-router";
import { ApprovalsPage } from "@/features/approvals";
import { listSearchSchema } from "@/shared/lib/search";

export const Route = createFileRoute("/_client/approvals")({
  validateSearch: listSearchSchema,
  staticData: { title: "Approvals" },
  component: () => {
    const search = Route.useSearch();
    return <ApprovalsPage search={search} />;
  },
});
