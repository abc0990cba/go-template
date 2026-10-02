import { createFileRoute } from "@tanstack/react-router";
import { TicketsPage, ticketsSearchSchema } from "@/features/tickets";

export const Route = createFileRoute("/_client/tickets")({
  validateSearch: ticketsSearchSchema,
  staticData: { title: "My Tickets" },
  component: () => {
    const search = Route.useSearch();
    return <TicketsPage search={search} />;
  },
});
