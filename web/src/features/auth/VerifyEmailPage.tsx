import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { CheckCircle2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { verifyEmail } from "./api";

export function VerifyEmailPage({ token }: { token?: string }) {
  const navigate = useNavigate();
  const [done, setDone] = useState(false);

  const verify = useMutation({
    mutationFn: (token: string) => verifyEmail(token),
    onSuccess: () => setDone(true),
    onError: (error) => toast.error(error.message),
  });

  useEffect(() => {
    if (token) verify.mutate(token);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {done ? <CheckCircle2 className="size-5 text-emerald-500" /> : null}
            Email verification
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {!token && (
            <>
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <XCircle className="size-4 text-destructive" /> This page needs a token — follow the
                link from the email.
              </p>
              <Button
                variant="outline"
                className="w-full"
                onClick={() => navigate({ to: "/login" })}
              >
                Back to login
              </Button>
            </>
          )}
          {token && !done && <p className="text-sm text-muted-foreground">Checking your link…</p>}
          {done && (
            <>
              <p className="text-sm text-muted-foreground">
                Your address is confirmed. Everything is set.
              </p>
              <Button asChild className="w-full">
                <Link to="/login">Go to login</Link>
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
