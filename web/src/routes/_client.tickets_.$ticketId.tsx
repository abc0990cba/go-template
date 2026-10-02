import { createFileRoute } from "@tanstack/react-router";
import { TicketDetailPage } from "@/features/tickets";

export const Route = createFileRoute("/_client/tickets_/$ticketId")({
  staticData: { title: "Ticket" },
  component: () => {
    const { ticketId } = Route.useParams();
    return <TicketDetailPage ticketId={ticketId} />;
  },
});
