import { useSuspenseQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { UserAvatar } from "@/components/user-avatar";
import { meQuery } from "@/shared/lib/auth";
import { useI18n } from "@/shared/lib/i18n";

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
    <div className="hidden items-center gap-2 rounded-full border py-1 pl-1 pr-2 xl:flex">
      <UserAvatar name={me.name} email={me.email} className="size-7" fallbackClassName="text-xs" />
      <div className="hidden leading-tight xl:block">
        <p className="max-w-36 truncate text-xs font-medium">{me.name || me.email}</p>
        <p className="max-w-36 truncate text-[10px] text-muted-foreground">{scope}</p>
      </div>
      <div className="hidden gap-1 xl:flex">
        {roles.map((role) => (
          <Badge key={role} variant="secondary" className="px-1.5 text-[10px]">
            {role}
          </Badge>
        ))}
      </div>
    </div>
  );
}
