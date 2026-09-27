import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { format } from "date-fns";
import { toast } from "sonner";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  Fingerprint,
  GitBranch,
  KeyRound,
  Loader2,
  LogOut,
  MailCheck,
  MonitorSmartphone,
  Trash2,
  Unlink,
} from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";
import { meQuery, oauthProvidersQuery } from "@/lib/auth";
import type { AuthIdentity } from "@/lib/types";
import { z } from "zod";

export const Route = createFileRoute("/_client/profile")({
  staticData: { title: "Profile" },
  validateSearch: z.object({ oauth_error: z.string().optional() }),
  component: ProfilePage,
});

function ProfilePage() {
  const { data: user } = useSuspenseQuery(meQuery());
  const { data: identities } = useSuspenseQuery({
    queryKey: ["me", "identities"],
    queryFn: api.myIdentities,
  });
  const { data: sessions } = useSuspenseQuery({
    queryKey: ["me", "sessions"],
    queryFn: api.sessions,
  });
  const { data: providers } = useQuery(oauthProvidersQuery());
  const { oauth_error: oauthError } = Route.useSearch();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const unlink = useMutation({
    mutationFn: api.unlinkProvider,
    onSuccess: () => {
      toast.success("Identity unlinked");
      refresh();
    },
    onError: (error) => toast.error(error.message),
  });

  const logoutAll = useMutation({
    mutationFn: api.logoutAll,
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
          "Link buttons render disabled when the provider has no client credentials configured (GOOGLE_CLIENT_ID / GITHUB_CLIENT_ID).",
          "“Log out everywhere” revokes every session row for your user; the list below is served by GET /api/sessions.",
        ]}
      >
        The account model is identity-based: a password is just the “password” provider, and
        Google/GitHub attach as additional identities on the same user. Sessions are opaque
        server-side tokens in PostgreSQL with a sliding 7-day expiry — reading <code>/api/me</code>{" "}
        with a valid cookie extends it.
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
                onUnlink={() => unlink.mutate(identity.provider)}
                unlinking={unlink.isPending}
              />
            ))}
            <div className="grid grid-cols-2 gap-2 pt-2">
              {(["google", "github"] as const).map((provider) => {
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
                    {provider === "google" ? <MailCheck /> : <GitBranch />}
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
    </div>
  );
}

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
        : GitBranch;
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
function APIKeysCard() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState("");
  const [secret, setSecret] = useState<string | null>(null);
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: api.listAPIKeys });

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["api-keys"] });

  const create = useMutation({
    mutationFn: () =>
      api.createAPIKey(
        name,
        scopes
          .split(/[\s,]+/)
          .map((scope: string) => scope.trim())
          .filter(Boolean),
      ),
    onSuccess: ({ secret: raw }) => {
      setSecret(raw);
      setName("");
      setScopes("");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeAPIKey(id),
    onSuccess: () => {
      toast.success("Key revoked");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>API keys</CardTitle>
        <CardDescription>
          Programmatic access with your permissions narrowed by scopes (e.g. users:read, jobs:*).
          Send the key as an Authorization: Bearer header.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {secret && (
          <div className="rounded border border-emerald-500/40 bg-emerald-500/10 p-2 text-xs">
            <p className="font-medium">Copy your key now — it is shown only once:</p>
            <code className="break-all font-mono">{secret}</code>
          </div>
        )}
        <div className="flex flex-wrap gap-2">
          <Input
            className="w-48"
            placeholder="Key name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <Input
            className="w-64"
            placeholder="scopes: users:read, jobs:*"
            value={scopes}
            onChange={(e) => setScopes(e.target.value)}
          />
          <Button size="sm" onClick={() => create.mutate()} disabled={!name || create.isPending}>
            Create key
          </Button>
        </div>
        <div className="space-y-1">
          {(keys.data ?? []).map((key) => (
            <div
              key={key.id}
              className="flex items-center gap-2 rounded border px-2 py-1.5 text-sm"
            >
              <span className="font-medium">{key.name}</span>
              <code className="text-xs text-muted-foreground">{key.prefix}…</code>
              {key.revoked_at ? (
                <Badge variant="secondary">revoked</Badge>
              ) : (
                <Badge variant="outline">active</Badge>
              )}
              <span className="text-xs text-muted-foreground">{key.scopes.join(", ")}</span>
              {!key.revoked_at && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="ml-auto size-6 text-destructive"
                  aria-label={`Revoke ${key.name}`}
                  onClick={() => revoke.mutate(key.id)}
                >
                  <Trash2 />
                </Button>
              )}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
