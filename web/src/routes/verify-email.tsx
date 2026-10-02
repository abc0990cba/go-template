import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { VerifyEmailPage } from "@/features/auth";

export const Route = createFileRoute("/verify-email")({
  validateSearch: z.object({ token: z.string().optional() }),
  staticData: { title: "Verify email" },
  component: () => {
    const { token } = Route.useSearch();
    return <VerifyEmailPage token={token} />;
  },
});
