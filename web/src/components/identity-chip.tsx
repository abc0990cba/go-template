import { useSuspenseQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { meQuery } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";

function initials(nameOrEmail: string) {
  const parts = nameOrEmail.split(/[\s@.]+/).filter(Boolean);
  const first = parts.at(0)?.charAt(0) ?? "?";
  const second = parts.at(1)?.charAt(0) ?? "";
  return (first + second).toUpperCase();
}

/**
 * IdentityChip answers "on whose behalf am I looking at the service":
 * avatar, name, the roles held in the active domain, and the scope label.
 * It lives in both shells — the client navbar and the admin console header —
 * so the acting identity is always one glance away.
 */
export function IdentityChip() {
  const { data: me } = useSuspenseQuery(meQuery());
  const { t } = useI18n();

  const activeOrg = localStorage.getItem("app.activeOrg") ?? "";
  const org = me.orgs.find((candidate) => candidate.id === activeOrg);
  const scope = org ? org.name : t("scope.platform");
  const roles = me.roles.slice(0, 2);

  return (
    <div className="flex items-center gap-2 rounded-full border py-1 pl-1 pr-3">
      <Avatar className="size-7">
        <AvatarFallback className="text-xs">{initials(me.name || me.email)}</AvatarFallback>
      </Avatar>
      <div className="hidden leading-tight sm:block">
        <p className="max-w-36 truncate text-xs font-medium">{me.name || me.email}</p>
        <p className="max-w-36 truncate text-[10px] text-muted-foreground">{scope}</p>
      </div>
      <div className="hidden gap-1 md:flex">
        {roles.map((role) => (
          <Badge key={role} variant="secondary" className="px-1.5 text-[10px]">
            {role}
          </Badge>
        ))}
      </div>
    </div>
  );
}
