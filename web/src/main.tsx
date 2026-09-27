import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "@/components/ui/sonner";
import { queryClient, router } from "@/lib/router";
import { ThemeProvider } from "next-themes";
import "@/styles/app.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("root element is missing");
}

// Theme values map 1:1 to the token blocks in styles/app.css; next-themes
// applies the chosen value as a class on <html>.
const THEMES = [
  "light",
  "dark",
  "vscode-light",
  "vscode-dark",
  "catppuccin-latte",
  "catppuccin-mocha",
] as const;

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        enableSystem
        themes={[...THEMES]}
        disableTransitionOnChange
      >
        <RouterProvider router={router} />
        <Toaster position="bottom-right" />
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
