import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { format } from "date-fns";
import { ArrowRight, CheckCheck, FileText, ShieldCheck } from "lucide-react";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import { useCan, useMe } from "@/shared/hooks/use-permissions";
import { TicketStatusBadge, ticketLabel, listTickets, ticketCounts } from "@/features/tickets";
import { approvals } from "@/features/approvals";
import { notifications as notificationsApi } from "@/features/notifications";
import { Alert, AlertDescription, AlertTitle } from "@/shared/components/ui/alert";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";

/**
 * The client dashboard is personal: my tickets, what waits on my approval,
 * my notifications. System-wide counters live in the /admin console — a
 * requester has no business seeing them. The public marketing landing owns
 * the `/` URL, so the authenticated home lives at /dashboard.
 */
export function ClientHomePage() {
  const me = useMe();
  const can = useCan();

  const counts = useQuery({
    queryKey: ["tickets", "counts", "my-home"],
    queryFn: () => ticketCounts("my"),
  });
  const recent = useQuery({
    queryKey: ["tickets", "recent-mine"],
    queryFn: () => listTickets({ mode: "offset", limit: 5 }),
  });
  const inboxEnabled = can("tickets", "approve");
  const inbox = useQuery({
    queryKey: ["approvals", "home"],
    queryFn: () => approvals({ mode: "offset", limit: 5 }),
    enabled: inboxEnabled,
  });
  const recentNotifications = useQuery({
    queryKey: ["notifications", "home"],
    queryFn: () => notificationsApi(5),
  });

  const statusChips = ["draft", "in_review", "approved", "completed", "rejected", "withdrawn"];

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">
          Welcome, {(me.name || me.email).split(" ")[0]}
        </h1>
        <p className="text-sm text-muted-foreground">
          Your personal workspace: file tickets, follow their approvals, deliver results.
        </p>
      </div>

      {can("admin_console", "access") && (
        <Alert className="border-primary/30">
          <ShieldCheck className="size-4" />
          <AlertTitle>You also have back-office access</AlertTitle>
          <AlertDescription>
            This is the client view — what ordinary accounts see.{" "}
            <Link to="/admin" className="underline">
              Open the admin console
            </Link>
            .
          </AlertDescription>
        </Alert>
      )}

      <ExplainerBanner
        title="How the ticket flow works"
        howTo={[
          "Create a ticket from a type — its approval chain decides who signs off, in which order.",
          "Submit it, then wait: each step's approver gets notified. You can withdraw at any moment, even after approval.",
          "If an approver rejects (with a comment), rework and resubmit; once approved, deliver the result to finish.",
        ]}
      >
        Your tickets live only in your list; nobody else's. The Approvals page appears in the navbar
        when you hold a role that signs off chains.
      </ExplainerBanner>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle className="text-base">My tickets</CardTitle>
              <CardDescription>
                {statusChips
                  .map((key) => `${key.replace("_", " ")}: ${counts.data?.[key] ?? 0}`)
                  .join(" · ")}
              </CardDescription>
            </div>
            <Link to="/tickets" className="text-sm font-medium underline underline-offset-4">
              All tickets
            </Link>
          </CardHeader>
          <CardContent>
            <ul className="divide-y text-sm">
              {(recent.data?.data ?? []).map((ticket) => (
                <li key={ticket.id} className="flex items-center justify-between gap-3 py-2">
                  <div className="min-w-0">
                    <Link
                      to="/tickets/$ticketId"
                      params={{ ticketId: ticket.id }}
                      className="flex items-center gap-2 underline-offset-4 hover:underline"
                    >
                      <span className="font-mono text-xs text-muted-foreground">
                        {ticketLabel(ticket.number)}
                      </span>
                      <span className="truncate font-medium">{ticket.title}</span>
                    </Link>
                    <p className="text-xs text-muted-foreground">
                      {ticket.type_key} · filed{" "}
                      {format(new Date(ticket.created_at), "MMM d, HH:mm")}
                    </p>
                  </div>
                  <TicketStatusBadge status={ticket.status} />
                </li>
              ))}
              {(recent.data?.data ?? []).length === 0 && (
                <li className="py-2 text-muted-foreground">
                  No tickets yet —{" "}
                  <Link to="/tickets" className="underline">
                    file the first one
                  </Link>
                  .
                </li>
              )}
            </ul>
          </CardContent>
        </Card>

        <div className="space-y-4">
          {inboxEnabled && (
            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <CardTitle className="flex items-center gap-2 text-base">
                  <CheckCheck className="size-4" /> Waiting for my decision
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm">
                {(inbox.data?.data ?? []).length === 0 ? (
                  <p className="text-muted-foreground">Nothing waits on your roles right now.</p>
                ) : (
                  (inbox.data?.data ?? []).map((ticket) => (
                    <Link
                      key={ticket.id}
                      to="/tickets/$ticketId"
                      params={{ ticketId: ticket.id }}
                      className="flex items-center justify-between gap-2 rounded-md border p-2 hover:bg-muted"
                    >
                      <span className="min-w-0 truncate">
                        <span className="font-mono text-xs">{ticketLabel(ticket.number)}</span>{" "}
                        {ticket.title}
                      </span>
                      <ArrowRight className="size-3.5 shrink-0 text-muted-foreground" />
                    </Link>
                  ))
                )}
                <Link
                  to="/approvals"
                  className="block text-xs font-medium text-muted-foreground underline underline-offset-4"
                >
                  Open the approvals inbox
                </Link>
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle className="flex items-center gap-2 text-base">
                <FileText className="size-4" /> Notifications
              </CardTitle>
              <Link
                to="/notifications"
                className="text-xs font-medium text-muted-foreground underline underline-offset-4"
              >
                All
              </Link>
            </CardHeader>
            <CardContent className="space-y-1.5 text-sm">
              {(recentNotifications.data ?? []).length === 0 ? (
                <p className="text-muted-foreground">Nothing yet.</p>
              ) : (
                (recentNotifications.data ?? []).slice(0, 4).map((entry) => (
                  <div key={entry.id} className="rounded-md border p-2">
                    <p className="font-medium">{entry.title}</p>
                    <p className="text-xs text-muted-foreground">
                      {format(new Date(entry.created_at), "MMM d, HH:mm")}
                    </p>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
