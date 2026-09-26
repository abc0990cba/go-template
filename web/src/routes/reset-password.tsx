import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";
import { Loader2 } from "lucide-react";
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

export const Route = createFileRoute("/reset-password")({
  validateSearch: z.object({ token: z.string().optional() }),
  staticData: { title: "Reset password" },
  component: ResetPasswordPage,
});

const form = z.object({
  password: z.string().min(8, "At least 8 characters"),
});

type FormValues = z.infer<typeof form>;

function ResetPasswordPage() {
  const navigate = useNavigate();
  const search = Route.useSearch();

  const reset = useMutation({
    mutationFn: (values: FormValues) => api.resetPassword(search.token ?? "", values.password),
    onSuccess: () => {
      toast.success("Password updated — sign in with the new one.");
      void navigate({ to: "/login" });
    },
    onError: (error) => toast.error(error.message),
  });

  const form2 = useForm<FormValues>({
    resolver: zodResolver(form),
    defaultValues: { password: "" },
  });

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Choose a new password</CardTitle>
          <CardDescription>
            The link is single-use; every existing session is signed out afterwards.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!search.token && (
            <p className="text-sm text-destructive">
              This page needs a token — follow the link from the email.
            </p>
          )}
          <Form {...form2}>
            <form
              onSubmit={form2.handleSubmit((values) => reset.mutate(values))}
              className="space-y-3"
            >
              <FormField
                control={form2.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>New password</FormLabel>
                    <FormControl>
                      <Input type="password" placeholder="••••••••" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button type="submit" className="w-full" disabled={reset.isPending || !search.token}>
                {reset.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Update
                password
              </Button>
            </form>
          </Form>
          <Link
            to="/login"
            className="block text-center text-sm text-muted-foreground hover:text-foreground"
          >
            Back to login
          </Link>
        </CardContent>
      </Card>
    </div>
  );
}
