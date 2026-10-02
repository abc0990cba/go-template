import { useQuery } from "@tanstack/react-query";

import { format } from "date-fns";
import { BadgeCheck, Check, Copy, Fingerprint, KeySquare, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { UserAvatar } from "@/shared/components/user-avatar";

import { Badge } from "@/shared/components/ui/badge";
import { Button } from "@/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";

import { Skeleton } from "@/shared/components/ui/skeleton";
import { userRoles } from "./api";

import { listRoles, rolePermissions } from "@/features/admin-rbac";

import type { User } from "@/shared/types/core";

export function UserHeroCard({
  user,
  roleRules,
  onManageRoles,
}: {
  user: User;
  roleRules: { object?: string; domain: string }[];
  onManageRoles: () => void;
}) {
  const [copied, setCopied] = useState("");

  const copy = (value: string, label: string) => {
    void navigator.clipboard?.writeText(value).then(
      () => {
        setCopied(label);
        toast.success(`${label} copied`);
        window.setTimeout(() => setCopied(""), 1500);
      },
      () => toast.error("Clipboard unavailable"),
    );
  };

  return (
    <Card className="gap-0 overflow-hidden py-0">
      {/* Cover band tinted from the same palette family as the avatar. */}
      <div className="relative h-20 bg-linear-to-r from-sky-500/15 via-violet-500/15 to-fuchsia-500/15 sm:h-24">
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top_right,theme(colors.sky.500/10),transparent_55%)]" />
      </div>
      <CardContent className="space-y-4 pb-5">
        <div className="-mt-10 flex flex-col gap-4 sm:-mt-12 sm:flex-row sm:items-end">
          <UserAvatar
            name={user.name}
            email={user.email}
            className="size-20 border-4 border-card shadow-md sm:size-24"
            fallbackClassName="text-2xl sm:text-3xl"
          />
          <div className="min-w-0 flex-1 pb-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-semibold tracking-tight sm:text-2xl">
                {user.name || "Unnamed user"}
              </h1>
              <Badge variant="secondary" className="gap-1 rounded-full">
                <BadgeCheck className="size-3" /> active
              </Badge>
            </div>
            <p className="truncate font-mono text-sm text-muted-foreground">{user.email}</p>
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {roleRules.length === 0 ? (
                <span className="text-xs text-muted-foreground">No roles assigned</span>
              ) : (
                roleRules.map((rule) => (
                  <Badge
                    key={`${rule.object}-${rule.domain}`}
                    variant="outline"
                    className="gap-1 rounded-full bg-primary/5"
                  >
                    <ShieldCheck className="size-3 text-violet-500" />
                    {rule.object}
                    {rule.domain !== "*" ? (
                      <span className="text-[10px] text-muted-foreground">
                        @{rule.domain.slice(0, 8)}
                      </span>
                    ) : null}
                  </Badge>
                ))
              )}
              <Button
                variant="ghost"
                size="sm"
                className="h-7 gap-1 px-2 text-xs text-muted-foreground"
                onClick={onManageRoles}
              >
                <ShieldCheck className="size-3" /> Manage
              </Button>
            </div>
          </div>
        </div>

        <div className="grid gap-x-6 gap-y-3 border-t pt-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <Fact label="User id" mono>
            <span className="truncate font-mono text-xs">{user.id}</span>
            <button
              type="button"
              aria-label="Copy user id"
              className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
              onClick={() => copy(user.id, "User id")}
            >
              {copied === "User id" ? (
                <Check className="size-3.5" />
              ) : (
                <Copy className="size-3.5" />
              )}
            </button>
          </Fact>
          <Fact label="Created">{format(new Date(user.created_at), "MMM d, yyyy HH:mm")}</Fact>
          <Fact label="Updated">{format(new Date(user.updated_at), "MMM d, yyyy HH:mm")}</Fact>
          <Fact label="External ref" mono>
            {user.external_ref ? (
              <>
                <span className="truncate font-mono text-xs">{user.external_ref}</span>
                <button
                  type="button"
                  aria-label="Copy external ref"
                  className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => copy(user.external_ref ?? "", "External ref")}
                >
                  {copied === "External ref" ? (
                    <Check className="size-3.5" />
                  ) : (
                    <Copy className="size-3.5" />
                  )}
                </button>
              </>
            ) : (
              <span className="text-xs text-muted-foreground">not set</span>
            )}
          </Fact>
        </div>
      </CardContent>
    </Card>
  );
}

export function Fact({
  label,
  mono = false,
  children,
}: {
  label: string;
  mono?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <p className="flex items-center gap-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
        {mono ? <Fingerprint className="size-3" /> : null}
        {label}
      </p>
      <div className={`mt-0.5 flex items-center gap-1 ${mono ? "min-w-0" : ""}`}>{children}</div>
    </div>
  );
}

/**
 * RolesPermissionsCard resolves each assigned role to its permission matrix:
 * role headers with domain badges, every object:action grant as a chip.
 */
export function RolesPermissionsCard({
  userId,
  className,
  onManage,
}: {
  userId: string;
  className?: string;
  onManage: () => void;
}) {
  const assignments = useQuery({
    queryKey: ["users", userId, "roles"],
    queryFn: () => userRoles(userId),
  });
  const catalog = useQuery({ queryKey: ["roles"], queryFn: listRoles });

  const roleKeys = [...new Set((assignments.data ?? []).map((rule) => rule.object ?? ""))].filter(
    Boolean,
  );

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeySquare className="size-4 text-muted-foreground" /> Roles &amp; permissions
            </CardTitle>
            <CardDescription>
              What this account may do: every grant behind the assigned roles.
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={onManage}>
            <ShieldCheck /> Manage
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {assignments.isPending ? (
          <div className="space-y-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-3/4" />
          </div>
        ) : roleKeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No roles assigned — this account has no explicit grants in this scope.
          </p>
        ) : (
          roleKeys.map((key) => <RoleBlock key={key} roleKey={key} catalog={catalog.data ?? []} />)
        )}
      </CardContent>
    </Card>
  );
}

export function RoleBlock({
  roleKey,
  catalog,
}: {
  roleKey: string;
  catalog: { key: string; name: string; description: string }[];
}) {
  const permissions = useQuery({
    queryKey: ["roles", roleKey, "permissions"],
    queryFn: () => rolePermissions(roleKey),
  });
  const meta = catalog.find((role) => role.key === roleKey);

  return (
    <div className="rounded-lg border p-3">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <p className="text-sm font-semibold">{meta?.name ?? roleKey}</p>
        <code className="text-[10px] text-muted-foreground">{roleKey}</code>
      </div>
      {meta?.description ? (
        <p className="mt-0.5 text-xs text-muted-foreground">{meta.description}</p>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-1">
        {permissions.isPending ? (
          <Skeleton className="h-5 w-40" />
        ) : (permissions.data?.length ?? 0) === 0 ? (
          <span className="text-xs text-muted-foreground">
            No permissions granted to this role.
          </span>
        ) : (
          permissions.data?.map((rule) => (
            <span
              key={`${rule.domain}-${rule.object}-${rule.action}`}
              className="rounded-md border bg-muted/50 px-1.5 py-0.5 font-mono text-[11px]"
              title={rule.domain === "*" ? "platform scope" : `org ${rule.domain}`}
            >
              {rule.object}:{rule.action}
            </span>
          ))
        )}
      </div>
    </div>
  );
}
