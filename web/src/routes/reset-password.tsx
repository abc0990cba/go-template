import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { ResetPasswordPage } from "@/features/auth";

export const Route = createFileRoute("/reset-password")({
  validateSearch: z.object({ token: z.string().optional() }),
  staticData: { title: "Reset password" },
  component: () => {
    const { token } = Route.useSearch();
    return <ResetPasswordPage token={token} />;
  },
});
