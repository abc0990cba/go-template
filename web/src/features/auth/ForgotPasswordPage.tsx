import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";
import { Loader2 } from "lucide-react";
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
import { forgotPassword } from "./api";

const form = z.object({
  email: z.string().email("Enter a valid email"),
});

type FormValues = z.infer<typeof form>;

export function ForgotPasswordPage() {
  const request = useMutation({
    mutationFn: (values: FormValues) => forgotPassword(values.email),
    onSuccess: () => {
      // Uniform answer regardless of account existence — no enumeration.
      toast.success("If that account exists, a reset link is on its way.");
    },
    onError: (error) => toast.error(error.message),
  });

  const form2 = useForm<FormValues>({
    resolver: zodResolver(form),
    defaultValues: { email: "" },
  });

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Forgot your password?</CardTitle>
          <CardDescription>
            Enter your email and we will send a reset link. In local development with Mailpit, read
            it at localhost:8025.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Form {...form2}>
            <form
              onSubmit={form2.handleSubmit((values) => request.mutate(values))}
              className="space-y-3"
            >
              <FormField
                control={form2.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Email</FormLabel>
                    <FormControl>
                      <Input type="email" placeholder="you@example.com" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button type="submit" className="w-full" disabled={request.isPending}>
                {request.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Send reset
                link
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
