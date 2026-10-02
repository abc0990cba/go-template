import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { LoginPage } from "@/features/auth";

const searchSchema = z.object({
  redirect: z.string().optional(),
});

export const Route = createFileRoute("/login")({
  validateSearch: searchSchema,
  staticData: { title: "Login" },
  component: () => {
    const { redirect } = Route.useSearch();
    return <LoginPage redirect={redirect} />;
  },
});
