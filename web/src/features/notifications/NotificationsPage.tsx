import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format } from "date-fns";
import { CheckCheck, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  markAllNotificationsRead,
  markNotificationRead,
  notifications as notificationsApi,
} from "./api";

export function NotificationsPage() {
  const queryClient = useQueryClient();
  const notifications = useQuery({
    queryKey: ["notifications", "all"],
    queryFn: () => notificationsApi(50),
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["notifications"] });
  };

  const markAll = useMutation({ mutationFn: markAllNotificationsRead, onSuccess: invalidate });
  const markOne = useMutation({
    mutationFn: (id: string) => markNotificationRead(id),
    onSuccess: invalidate,
  });

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Notifications</h1>
          <p className="text-sm text-muted-foreground">
            Job outcomes, grants, and security events addressed to you.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="gap-2"
          onClick={() => markAll.mutate()}
          disabled={markAll.isPending}
        >
          <CheckCheck className="size-4" /> Mark all read
        </Button>
      </div>

      <div className="space-y-1">
        {notifications.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {(notifications.data ?? []).map((notification) => (
          <div
            key={notification.id}
            className={
              "flex items-start gap-3 rounded-lg border px-3 py-2.5 " +
              (notification.read_at ? "opacity-70" : "border-l-4 border-l-primary")
            }
          >
            <div className="min-w-0 flex-1">
              <p
                className={
                  notification.read_at ? "text-sm text-muted-foreground" : "text-sm font-medium"
                }
              >
                {notification.title}
              </p>
              {notification.body && (
                <p className="text-xs text-muted-foreground">{notification.body}</p>
              )}
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {format(new Date(notification.created_at), "MMM d, yyyy HH:mm")}
              </p>
            </div>
            {!notification.read_at && (
              <Button
                variant="ghost"
                size="icon"
                className="size-7"
                aria-label="Mark read"
                onClick={() => markOne.mutate(notification.id)}
              >
                <Check />
              </Button>
            )}
          </div>
        ))}
        {!notifications.isPending && (notifications.data ?? []).length === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">
            Nothing here yet — queue a job or wait for someone to grant you a role.
          </p>
        )}
      </div>
    </div>
  );
}
