import { createFileRoute } from "@tanstack/react-router";
import { AdminUserDetailPage } from "@/features/admin-users";

export const Route = createFileRoute("/admin/users_/$userId")({
  staticData: { title: "User" },
  component: () => {
    const { userId } = Route.useParams();
    return <AdminUserDetailPage userId={userId} />;
  },
});
