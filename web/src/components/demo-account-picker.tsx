import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { LogIn } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { api } from "@/lib/api";
import { meQuery } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { demoAccounts, type DemoAccount } from "@/lib/demo-accounts";

/**
 * DemoAccountPicker is the interactive accounts table: every seeded account
 * with its roles, what to check, and a one-click sign-in that swaps the
 * session. Shown on the login page (when the demo_mode flag is on) and in
 * the "switch account" dialogs of both shells.
 */
export function DemoAccountPicker() {
  const { locale, t } = useI18n();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [pendingEmail, setPendingEmail] = useState("");

  const signIn = useMutation({
    mutationFn: async (account: DemoAccount) => {
      await api.login(account.email, account.password);
      // Drop every cached query (the old identity's data), then resolve the
      // fresh session before navigating so guards see the new roles.
      queryClient.clear();
      await queryClient.fetchQuery(meQuery());
      return account;
    },
    onSuccess: (account) => {
      toast.success(`${t("demo.signedInAs")} ${account.name}`);
      setPendingEmail("");
      void navigate({ to: account.area === "admin" ? "/admin" : "/" });
    },
    onError: (error) => {
      setPendingEmail("");
      toast.error(error.message);
    },
  });

  return (
    <div className="overflow-hidden rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("demo.account")}</TableHead>
            <TableHead>{t("demo.roles")}</TableHead>
            <TableHead className="hidden md:table-cell">{t("demo.password")}</TableHead>
            <TableHead className="hidden md:table-cell">{t("demo.whatToCheck")}</TableHead>
            <TableHead className="text-right">{t("demo.area")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {demoAccounts.map((account) => (
            <TableRow key={account.email}>
              <TableCell>
                <p className="font-medium">{account.name}</p>
                <p className="font-mono text-xs text-muted-foreground">{account.email}</p>
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {account.roles.map((role) => (
                    <Badge key={role} variant="secondary" className="text-[10px]">
                      {role}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell className="hidden md:table-cell">
                <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{account.password}</code>
              </TableCell>
              <TableCell className="hidden max-w-72 text-xs text-muted-foreground md:table-cell">
                {locale === "ru" ? account.whatToCheckRu : account.whatToCheck}
              </TableCell>
              <TableCell className="text-right">
                <Badge variant={account.area === "admin" ? "default" : "outline"} className="mr-2">
                  {account.area === "admin" ? t("demo.areaAdmin") : t("demo.areaClient")}
                </Badge>
                <Button
                  size="sm"
                  variant="outline"
                  className="gap-1"
                  disabled={pendingEmail !== ""}
                  onClick={() => {
                    setPendingEmail(account.email);
                    signIn.mutate(account);
                  }}
                >
                  <LogIn /> {t("demo.signin")}
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
