import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { toast } from "sonner";
import { useQuery } from "@tanstack/react-query";
import {
  AtSign,
  Fingerprint,
  GitBranch,
  KeyRound,
  Loader2,
  LogOut,
  MailCheck,
  MonitorSmartphone,
  ThumbsUp,
  Unlink,
} from "lucide-react";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import { Avatar, AvatarFallback } from "@/shared/components/ui/avatar";
import { Button } from "@/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";
import { meQuery } from "@/shared/lib/auth";
import {
  logoutAll as logoutAllApi,
  myIdentities,
  sessions as fetchSessions,
  unlinkProvider,
  oauthProviders,
} from "./api";
import { APIKeysCard } from "./ProfileApiKeysCard";
import { MFACard } from "./ProfileMfaCard";
import type { AuthIdentity } from "./types";

export function ProfilePage({ oauthError }: { oauthError?: string }) {
  const { data: user } = useSuspenseQuery(meQuery());
  const { data: identities } = useSuspenseQuery({
    queryKey: ["me", "identities"],
    queryFn: myIdentities,
  });
  const { data: sessions } = useSuspenseQuery({
    queryKey: ["me", "sessions"],
    queryFn: fetchSessions,
  });
  const { data: providers } = useQuery({
    queryKey: ["oauth", "providers"],
    queryFn: oauthProviders,
    staleTime: 5 * 60_000,
  });
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const unlink = useMutation({
    mutationFn: unlinkProvider,
    onSuccess: () => {
      toast.success("Identity unlinked");
      refresh();
    },
    onError: (error) => toast.error(error.message),
  });

  const logoutAll = useMutation({
    mutationFn: logoutAllApi,
    onSuccess: () => {
      queryClient.removeQueries();
      void navigate({ to: "/login" });
    },
    onError: (error) => toast.error(error.message),
  });

  const configured = new Map(
    (providers ?? []).map((provider) => [provider.name, provider.configured]),
  );
  const linked = new Set(
    identities.filter((identity) => identity.provider !== "password").map((i) => i.provider),
  );

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Profile</h1>
        <p className="text-sm text-muted-foreground">
          Your account, linked identity providers, and active sessions.
        </p>
      </div>

      {oauthError ? (
        <AlertDismissible message={decodeURIComponent(oauthError).replace(/\+/g, " ")} />
      ) : null}

      <Card>
        <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <Avatar className="size-12">
            <AvatarFallback className="text-lg">{initial(user.name || user.email)}</AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <CardTitle>{user.name || "—"}</CardTitle>
            <CardDescription className="font-mono break-all">{user.email}</CardDescription>
          </div>
          <div className="sm:ml-auto">
            <Button
              variant="destructive"
              size="sm"
              onClick={() => logoutAll.mutate()}
              disabled={logoutAll.isPending}
            >
              {logoutAll.isPending ? <Loader2 className="size-4 animate-spin" /> : <LogOut />}
              Log out everywhere
            </Button>
          </div>
        </CardHeader>
      </Card>

      <ExplainerBanner
        title="Identities and sessions"
        howTo={[
          "Link a provider with the buttons below — the redirect goes to the backend, which performs the code exchange and returns to this page.",
          "Link buttons render disabled when the provider has no client credentials configured (GOOGLE_CLIENT_ID, GITHUB_CLIENT_ID, FACEBOOK_CLIENT_ID, TWITTER_CLIENT_ID).",
          "“Log out everywhere” revokes every session row for your user; the list below is served by GET /api/sessions.",
        ]}
      >
        The account model is identity-based: a password is just the “password” provider, and
        Google/GitHub/Facebook/Twitter attach as additional identities on the same user. Sessions
        are opaque server-side tokens in PostgreSQL with a sliding 7-day expiry — reading{" "}
        <code>/api/me</code> with a valid cookie extends it.
      </ExplainerBanner>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Linked identities</CardTitle>
            <CardDescription>
              {identities.length} identity record(s) on this account.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {identities.map((identity) => (
              <IdentityRow
                key={identity.provider + identity.provider_subject}
                identity={identity}
                onUnlink={() => {
                  // The server detaches OAuth providers only; password is a credential.
                  if (identity.provider !== "password") unlink.mutate(identity.provider);
                }}
                unlinking={unlink.isPending}
              />
            ))}
            <div className="grid grid-cols-2 gap-2 pt-2">
              {(["google", "github", "facebook", "twitter"] as const).map((provider) => {
                const isConfigured = configured.get(provider) ?? false;
                const isLinked = linked.has(provider);
                return (
                  <Button
                    key={provider}
                    variant="outline"
                    size="sm"
                    disabled={!isConfigured || isLinked}
                    onClick={() => {
                      // Server-side redirect flow: /auth/:provider/login.
                      window.location.href = `/auth/${provider}/login`;
                    }}
                    title={
                      !isConfigured
                        ? `${provider} client credentials are not configured on the backend`
                        : isLinked
                          ? "Already linked"
                          : `Link ${provider}`
                    }
                  >
                    {provider === "google" ? (
                      <MailCheck />
                    ) : provider === "github" ? (
                      <GitBranch />
                    ) : provider === "facebook" ? (
                      <ThumbsUp />
                    ) : (
                      <AtSign />
                    )}
                    {isLinked ? `${provider} linked` : `Link ${provider}`}
                  </Button>
                );
              })}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <MonitorSmartphone className="size-4" /> Active sessions
            </CardTitle>
            <CardDescription>
              Sessions are opaque tokens; ids show a short prefix only.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2 text-sm">
              {sessions.map((session) => (
                <li
                  key={session.id}
                  className="flex items-center justify-between border-b pb-2 last:border-0"
                >
                  <div>
                    <p className="font-mono text-xs">{session.id}…</p>
                    <p className="text-xs text-muted-foreground">
                      Expires {format(new Date(session.expires_at), "MMM d, HH:mm")}
                    </p>
                  </div>
                  {session.current ? (
                    <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
                      this session
                    </span>
                  ) : (
                    <span className="text-xs text-muted-foreground">other device</span>
                  )}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>
      <APIKeysCard />
      <MFACard />
    </div>
  );
}

/**
 * MFACard runs the TOTP lifecycle: enroll (secret + QR), confirm with a code
 * and receive recovery codes once, and disable with password + code. The
 * recovery codes render exactly once, like API key secrets.
 */
function IdentityRow({
  identity,
  onUnlink,
  unlinking,
}: {
  identity: AuthIdentity;
  onUnlink: () => void;
  unlinking: boolean;
}) {
  const Icon =
    identity.provider === "password"
      ? KeyRound
      : identity.provider === "google"
        ? MailCheck
        : identity.provider === "github"
          ? GitBranch
          : identity.provider === "facebook"
            ? ThumbsUp
            : AtSign;
  return (
    <div className="flex items-center gap-3 rounded-md border p-2">
      <Fingerprint className="ml-1 size-4 text-muted-foreground" />
      <Icon className="size-4" />
      <div className="min-w-0">
        <p className="text-sm font-medium capitalize">{identity.provider}</p>
        <p className="truncate text-xs text-muted-foreground">
          {identity.email || identity.provider_subject}
        </p>
      </div>
      <div className="ml-auto flex items-center gap-2">
        {identity.provider !== "password" ? (
          <Button variant="ghost" size="sm" onClick={onUnlink} disabled={unlinking}>
            <Unlink /> Unlink
          </Button>
        ) : (
          <span className="text-xs text-muted-foreground">credential</span>
        )}
      </div>
    </div>
  );
}

function AlertDismissible({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400">
      {message}
    </div>
  );
}

function initial(text: string) {
  return text.charAt(0).toUpperCase();
}

/**
 * APIKeysCard issues and revokes personal API keys. The secret renders
 * exactly once — right after creation — and never leaves the browser again.
 */
