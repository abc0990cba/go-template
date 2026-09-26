import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate, type FileRouteTypes } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { Link } from "@tanstack/react-router";
import { z } from "zod";
import { Eye, EyeOff, Loader2 } from "lucide-react";
import { useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { meQuery } from "@/lib/auth";

const searchSchema = z.object({
  redirect: z.string().optional(),
});

export const Route = createFileRoute("/login")({
  validateSearch: searchSchema,
  staticData: { title: "Login" },
  component: LoginPage,
});

const loginForm = z.object({
  email: z.string().min(3, "Enter an email"),
  password: z.string().min(8, "At least 8 characters"),
});

type LoginForm = z.infer<typeof loginForm>;

function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const search = Route.useSearch();
  const [showPassword, setShowPassword] = useState(false);

  const form = useForm<LoginForm>({
    resolver: zodResolver(loginForm),
    defaultValues: { email: "demo@go-template.local", password: "demo-password-123" },
  });

  const login = useMutation({
    mutationFn: (values: LoginForm) => api.login(values.email, values.password),
    onSuccess: ({ user }) => {
      // The session probe now carries the RBAC context; refetch it fresh.
      queryClient.setQueryData(meQuery().queryKey, {
        ...user,
        orgs: [],
        active_org: "*",
        roles: [],
        permissions: [],
        impersonated: false,
      });
      const fallback: FileRouteTypes["to"] = "/";
      const target = (search.redirect ?? fallback) as FileRouteTypes["to"];
      void navigate({ to: target.startsWith("/_") || target.includes("?") ? fallback : target });
    },
    onError: (error) => {
      toast.error(error.message);
    },
  });

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <div className="w-full max-w-sm space-y-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-xl">Go Template Admin</CardTitle>
            <CardDescription>Sign in to manage users, jobs, and analytics.</CardDescription>
          </CardHeader>
          <CardContent>
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
                      <FormLabel>Password</FormLabel>
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
                            className="absolute inset-y-0 right-0 flex items-center px-3 text-muted-foreground"
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
                <Button type="submit" className="w-full" disabled={login.isPending}>
                  {login.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                  Sign in
                </Button>
              </form>
            </Form>
            <Link
              to="/forgot-password"
              className="text-center text-sm text-muted-foreground hover:text-foreground"
            >
              Forgot your password?
            </Link>
            <p className="mt-4 text-center text-sm text-muted-foreground">
              No account?{" "}
              <a href="/register" className="font-medium underline underline-offset-4">
                Register
              </a>
            </p>
          </CardContent>
        </Card>

        <Alert>
          <AlertTitle>Demo credentials</AlertTitle>
          <AlertDescription>
            The pre-filled <code>demo@go-template.local</code> / <code>demo-password-123</code>{" "}
            account comes from the demo seed (APP_DEMO_EMAIL / APP_DEMO_PASSWORD). Ten more demo
            users (<code>demo-u-0001@demo.example</code> …<code>demo-u-0010@demo.example</code>,
            password <code>password123</code>) work once the demo dataset is seeded.
          </AlertDescription>
        </Alert>
      </div>
    </div>
  );
}
