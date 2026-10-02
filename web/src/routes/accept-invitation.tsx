import { createFileRoute } from "@tanstack/react-router";
import { AcceptInvitationPage } from "@/features/auth";

export const Route = createFileRoute("/accept-invitation")({
  validateSearch: (search: Record<string, unknown>): { token?: string } => ({
    token: typeof search.token === "string" ? search.token : undefined,
  }),
  staticData: { title: "Accept invitation" },
  component: () => {
    const { token } = Route.useSearch();
    return <AcceptInvitationPage token={token} />;
  },
});
