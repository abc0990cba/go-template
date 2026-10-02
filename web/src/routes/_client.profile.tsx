import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { ProfilePage } from "@/features/profile";

export const Route = createFileRoute("/_client/profile")({
  validateSearch: z.object({ oauth_error: z.string().optional() }),
  staticData: { title: "Profile" },
  component: () => {
    const { oauth_error } = Route.useSearch();
    return <ProfilePage oauthError={oauth_error} />;
  },
});
