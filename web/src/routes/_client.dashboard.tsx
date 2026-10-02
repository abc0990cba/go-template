import { createFileRoute } from "@tanstack/react-router";
import { ClientHomePage } from "@/features/home";

export const Route = createFileRoute("/_client/dashboard")({
  staticData: { title: "Dashboard" },
  component: ClientHomePage,
});
