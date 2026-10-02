import { createFileRoute } from "@tanstack/react-router";
import { TicketTypesAdminPage } from "@/features/tickets";

export const Route = createFileRoute("/admin/ticket-types")({
  staticData: { title: "Ticket Types" },
  component: TicketTypesAdminPage,
});
