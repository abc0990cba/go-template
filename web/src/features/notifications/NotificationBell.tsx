import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Bell, CheckCheck } from "lucide-react";
import { Button } from "@/shared/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/components/ui/dropdown-menu";
import { markAllNotificationsRead, markNotificationRead, notifications, unreadCount } from "./api";
import { formatDistanceToNow } from "date-fns";

/**
 * NotificationBell is the header entry point: an unread badge fed by the
 * unread-count query (invalidated live by the SSE provider) and a dropdown
 * with the newest records.
 */
export function NotificationBell() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const unread = useQuery({
    queryKey: ["notifications", "unread"],
    queryFn: unreadCount,
    refetchInterval: 60_000,
  });
  const recent = useQuery({
    queryKey: ["notifications", "recent"],
    queryFn: () => notifications(8),
    refetchInterval: 60_000,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["notifications"] });
  };

  const markAll = useMutation({
    mutationFn: markAllNotificationsRead,
    onSuccess: invalidate,
  });
  const markOne = useMutation({
    mutationFn: (id: string) => markNotificationRead(id),
    onSuccess: invalidate,
  });

  const count = unread.data?.count ?? 0;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="relative" aria-label="Notifications">
          <Bell className="size-4" />
          {count > 0 && (
            <span className="absolute -right-0.5 -top-0.5 flex size-4 items-center justify-center rounded-full bg-destructive text-[10px] font-semibold text-white">
              {count > 9 ? "9+" : count}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <div className="flex items-center justify-between px-2 py-1.5">
          <p className="text-sm font-semibold">Notifications</p>
          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1 text-xs text-muted-foreground md:h-6"
            onClick={() => markAll.mutate()}
            disabled={count === 0}
          >
            <CheckCheck className="size-3" /> Mark all read
          </Button>
        </div>
        <DropdownMenuSeparator />
        {(recent.data ?? []).length === 0 && (
          <p className="px-2 py-3 text-sm text-muted-foreground">
            Nothing yet — you are all caught up.
          </p>
        )}
        {(recent.data ?? []).map((notification) => (
          <DropdownMenuItem
            key={notification.id}
            className="flex-col items-start gap-0.5 py-2"
            onClick={() => {
              if (!notification.read_at) markOne.mutate(notification.id);
              if (notification.link) void navigate({ to: notification.link });
            }}
          >
            <span className="flex w-full items-center gap-2">
              {!notification.read_at && (
                <span className="size-1.5 shrink-0 rounded-full bg-primary" />
              )}
              <span
                className={
                  notification.read_at ? "truncate text-muted-foreground" : "truncate font-medium"
                }
              >
                {notification.title}
              </span>
            </span>
            <span className="pl-3.5 text-xs text-muted-foreground">
              {formatDistanceToNow(new Date(notification.created_at), { addSuffix: true })}
            </span>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link to="/notifications" className="w-full text-sm">
            View all
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
