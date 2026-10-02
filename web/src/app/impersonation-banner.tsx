import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Button } from "@/shared/components/ui/button";
import { useI18n } from "@/shared/lib/i18n";
import { meQuery } from "@/shared/lib/auth";
import { stopImpersonation } from "@/features/admin-users";

/**
 * ImpersonationBanner sits above everything when an admin is viewing the
 * app as someone else; stopping returns them to their own session.
 */
export function ImpersonationBanner() {
  const { data: me } = useSuspenseQuery(meQuery());
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const stop = useMutation({
    mutationFn: stopImpersonation,
    onSuccess: () => void queryClient.invalidateQueries(),
  });
  if (!me.impersonated) return null;
  return (
    <div className="sticky top-0 z-50 flex flex-wrap items-center justify-center gap-x-3 gap-y-1 bg-amber-500 px-4 py-1.5 text-center text-sm font-medium text-black">
      <span className="min-w-0 truncate">
        {t("banner.impersonating")} {me.email} ({me.impersonator_name ?? "admin"})
      </span>
      <Button
        size="sm"
        variant="outline"
        className="h-8 md:h-6"
        onClick={() => stop.mutate()}
        disabled={stop.isPending}
      >
        {t("banner.returnToAdmin")}
      </Button>
    </div>
  );
}
