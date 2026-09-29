import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { Loader2, MailPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api } from "@/lib/api";
import { meQuery } from "@/lib/auth";

interface AcceptSearch {
  token?: string;
}

export const Route = createFileRoute("/accept-invitation")({
  validateSearch: (search: Record<string, unknown>): AcceptSearch => ({
    token: typeof search.token === "string" ? search.token : undefined,
  }),
  staticData: { title: "Accept invitation" },
  component: AcceptInvitationPage,
});

/**
 * The landing page behind invitation emails: the token arrives in the URL,
 * the caller must be signed in (the org is granted to *their* account), and
 * acceptance adds the membership + domain role in one call.
 */
function AcceptInvitationPage() {
  const { token } = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const accept = useMutation({
    mutationFn: () => api.acceptInvitation(token ?? ""),
    onSuccess: async ({ org_id }) => {
      toast.success("Invitation accepted — welcome to the organization");
      queryClient.clear();
      await queryClient.fetchQuery(meQuery());
      void navigate({ to: "/", search: { org: org_id } });
    },
    onError: (error) => toast.error(error.message),
  });

  const signInFirst = async () => {
    void navigate({ to: "/login", search: { redirect: `/accept-invitation?token=${token ?? ""}` } });
  };

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="gap-3">
          <span className="flex size-11 items-center justify-center rounded-xl bg-primary/10">
            <MailPlus className="size-5 text-primary" />
          </span>
          <CardTitle className="text-xl">Organization invitation</CardTitle>
          <CardDescription>
            {token
              ? "Accepting joins your account to the organization with the invited role."
              : "This page needs an invitation token — use the link from your email."}
          </CardDescription>
        </CardHeader>
        <CardFooter className="flex gap-2">
          <Button onClick={() => accept.mutate()} disabled={!token || accept.isPending}>
            {accept.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Accept invitation
          </Button>
          <Button variant="outline" onClick={() => void signInFirst()}>
            Sign in first
          </Button>
        </CardFooter>
        {accept.isError ? (
          <CardContent>
            <p className="text-sm text-destructive">
              The invitation could not be accepted — it may be revoked, expired, or already used.
            </p>
          </CardContent>
        ) : null}
      </Card>
    </div>
  );
}
