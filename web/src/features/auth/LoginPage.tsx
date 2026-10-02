import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { Link } from "@tanstack/react-router";
import { z } from "zod";
import { Eye, EyeOff, KeySquare, Loader2, ScrollText, ShieldCheck, Users } from "lucide-react";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Alert, AlertDescription, AlertTitle } from "@/shared/components/ui/alert";
import { Button } from "@/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/shared/components/ui/form";
import { Input } from "@/shared/components/ui/input";
import { loginStep, mfaChallenge } from "./api";
import { api, unwrap } from "@/shared/api/core";
import { meQuery } from "@/shared/lib/auth";
import { DemoAccountPicker } from "./DemoAccountPicker";
import { I18nProvider } from "@/shared/lib/i18n";

const loginForm = z.object({
  email: z.string().min(3, "Enter an email"),
  password: z.string().min(8, "At least 8 characters"),
});

type LoginForm = z.infer<typeof loginForm>;

export function LoginPage({ redirect }: { redirect?: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [showPassword, setShowPassword] = useState(false);
  // When the password step answers mfa_required, the card switches to the
  // code step: the same form asks for the 6-digit TOTP or a recovery code.
  const [challenge, setChallenge] = useState<string | null>(null);
  const [code, setCode] = useState("");

  const form = useForm<LoginForm>({
    resolver: zodResolver(loginForm),
    defaultValues: { email: "demo@go-template.local", password: "demo-password-123" },
  });

  const finishLogin = async () => {
    // The login response carries no RBAC context; drop everything cached
    // under the previous identity and resolve the real session probe
    // (roles, permissions, orgs) before the redirect, so the route guards
    // judge the new account by its actual permissions.
    queryClient.clear();
    await queryClient.fetchQuery(meQuery());
    // Same-origin paths only, query string intact: the client area keeps its
    // table state in the URL, so a deep link back to
    // /tickets?mode=cursor&view=paged&page=2 must restore exactly that view.
    // External URLs and protocol-relative //host fall back to the dashboard.
    const requested = redirect ?? "";
    const target =
      requested.startsWith("/") && !requested.startsWith("//") ? requested : "/dashboard";
    void navigate({ href: target });
  };

  const login = useMutation({
    mutationFn: (values: LoginForm) => loginStep(values.email, values.password),
    onSuccess: async (outcome) => {
      if (outcome.kind === "mfa_required") {
        setChallenge(outcome.challenge);
        setCode("");
        return;
      }
      if (outcome.kind === "enrollment_required") {
        toast.error(
          "Two-factor authentication is required for your account. Sign in is blocked until you set it up — ask an administrator to reset your enrollment.",
        );
        return;
      }
      await finishLogin();
    },
    onError: (error) => {
      toast.error(error.message);
    },
  });

  const challengeMutation = useMutation({
    mutationFn: () => mfaChallenge(challenge ?? "", code.trim()),
    onSuccess: async () => {
      setChallenge(null);
      await finishLogin();
    },
    onError: (error) => {
      toast.error(error.message);
    },
  });

  // The demo_mode flag (a public setting) turns the login page into a
  // role-switching console: every seeded account signs in with one click.
  const demo = useQuery({
    queryKey: ["settings", "public"],
    queryFn: () => unwrap(api.GET("/api/settings/public")),
    staleTime: 5 * 60_000,
  });
  const demoMode = demo.data?.demo_mode === "true";

  return (
    <div className="relative flex min-h-svh items-center justify-center overflow-hidden p-4 sm:p-6 md:p-8">
      {/* Decorative background washes — subtle on purpose, they only tint
          the corners so the cards stay the focus. */}
      <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
        <div className="absolute inset-0 bg-[radial-gradient(48rem_32rem_at_115%_-10%,rgba(139,92,246,0.13),transparent)]" />
        <div className="absolute inset-0 bg-[radial-gradient(42rem_30rem_at_-15%_110%,rgba(14,165,233,0.12),transparent)]" />
      </div>

      <div className="grid w-full max-w-6xl grid-cols-1 items-start gap-6 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
        <div className="space-y-4">
          <Card className="shadow-xl shadow-primary/5">
            <CardHeader className="gap-4">
              <div className="flex items-center gap-3">
                <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-sky-500 to-violet-600 text-lg font-bold text-white shadow-md">
                  G
                </span>
                <div className="min-w-0 flex-1">
                  <CardTitle className="text-xl">Go Template Admin</CardTitle>
                  <CardDescription>Sign in to manage users, jobs, and analytics.</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {challenge ? (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    challengeMutation.mutate();
                  }}
                  className="space-y-4"
                >
                  <div className="flex items-center gap-2 text-sm">
                    <ShieldCheck className="size-4 text-primary" />
                    <span>Two-factor authentication is on for this account.</span>
                  </div>
                  <div className="space-y-2">
                    <label htmlFor="mfa-code" className="text-sm font-medium">
                      Authentication code
                    </label>
                    <Input
                      id="mfa-code"
                      inputMode="text"
                      autoComplete="one-time-code"
                      placeholder="123456 or recovery code"
                      value={code}
                      onChange={(event) => setCode(event.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      Enter the 6-digit code from your authenticator app, or one of your recovery
                      codes.
                    </p>
                  </div>
                  <Button
                    type="submit"
                    className="w-full bg-linear-to-r from-sky-600 to-violet-600 text-white hover:from-sky-500 hover:to-violet-500"
                    disabled={code.trim().length === 0 || challengeMutation.isPending}
                  >
                    {challengeMutation.isPending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : null}
                    Verify and sign in
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    className="w-full"
                    onClick={() => {
                      setChallenge(null);
                      setCode("");
                    }}
                  >
                    Back
                  </Button>
                </form>
              ) : (
                <Form {...form}>
                  <form
                    onSubmit={form.handleSubmit((values) => login.mutate(values))}
                    className="space-y-4"
                  >
                    <FormField
                      control={form.control}
                      name="email"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Email</FormLabel>
                          <FormControl>
                            <Input
                              type="email"
                              inputMode="email"
                              autoComplete="email"
                              placeholder="you@example.com"
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="password"
                      render={({ field }) => (
                        <FormItem>
                          <div className="flex items-center justify-between">
                            <FormLabel>Password</FormLabel>
                            <Link
                              to="/forgot-password"
                              className="text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
                            >
                              Forgot password?
                            </Link>
                          </div>
                          <FormControl>
                            <div className="relative">
                              <Input
                                type={showPassword ? "text" : "password"}
                                autoComplete="current-password"
                                placeholder="••••••••"
                                className="pr-10"
                                {...field}
                              />
                              <button
                                type="button"
                                onClick={() => setShowPassword((previous) => !previous)}
                                className="absolute inset-y-0 right-0 flex w-10 items-center justify-center text-muted-foreground"
                                aria-label={showPassword ? "Hide password" : "Show password"}
                              >
                                {showPassword ? (
                                  <EyeOff className="size-4" />
                                ) : (
                                  <Eye className="size-4" />
                                )}
                              </button>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <Button
                      type="submit"
                      className="w-full bg-linear-to-r from-sky-600 to-violet-600 text-white hover:from-sky-500 hover:to-violet-500"
                      disabled={login.isPending}
                    >
                      {login.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                      Sign in
                    </Button>
                  </form>
                </Form>
              )}
              <p className="mt-4 text-center text-sm text-muted-foreground">
                No account?{" "}
                <a href="/register" className="font-medium underline underline-offset-4">
                  Register
                </a>
              </p>
            </CardContent>
          </Card>

          {!demoMode && (
            <Alert>
              <KeySquare />
              <AlertTitle>Demo credentials</AlertTitle>
              <AlertDescription>
                The pre-filled <code className="whitespace-nowrap">demo@go-template.local</code> /{" "}
                <code className="whitespace-nowrap">demo-password-123</code> account comes from the
                demo seed (APP_DEMO_EMAIL / APP_DEMO_PASSWORD).
              </AlertDescription>
            </Alert>
          )}
        </div>

        {demoMode ? (
          <I18nProvider>
            <div className="min-w-0 space-y-3">
              <div>
                <h2 className="text-lg font-semibold tracking-tight">Demo accounts</h2>
                <p className="text-sm text-muted-foreground">
                  One click signs you in as that role — walk the whole ticket flow from the
                  requester to the approver without typing passwords.
                </p>
              </div>
              <DemoAccountPicker />
            </div>
          </I18nProvider>
        ) : (
          <BrandPanel />
        )}
      </div>
    </div>
  );
}

/** Feature panel filling the right column when the demo picker is off. */
function BrandPanel() {
  const features = [
    {
      icon: Users,
      title: "Users, roles & permissions",
      text: "Full RBAC with Casbin, per-tenant grants, and a complete audit trail of every change.",
    },
    {
      icon: ScrollText,
      title: "Background jobs & event streams",
      text: "Queued work with live status, Kafka/NATS enrichment, and a dead-letter console.",
    },
    {
      icon: ShieldCheck,
      title: "Production shape",
      text: "OpenAPI contract, metrics, traces, and resilience tooling — all in one binary.",
    },
  ];
  return (
    <div className="hidden min-w-0 space-y-6 self-center lg:block">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">
          One service, the whole platform shape
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          A deliberately small Go + React playground that behaves like a real product.
        </p>
      </div>
      <ul className="space-y-4">
        {features.map((feature) => (
          <li key={feature.title} className="flex gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border bg-card shadow-xs">
              <feature.icon className="size-4.5 text-primary" />
            </span>
            <div>
              <p className="text-sm font-medium">{feature.title}</p>
              <p className="text-sm text-muted-foreground">{feature.text}</p>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
