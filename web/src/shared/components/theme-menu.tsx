import { useTheme } from "next-themes";
import { Check, Monitor, Moon, Palette, Sun } from "lucide-react";
import { Button } from "@/shared/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/components/ui/dropdown-menu";

interface ThemeOption {
  value: string;
  label: string;
  hint: string;
  icon: typeof Sun;
  /** [background, accent] preview swatch, hex so it does not depend on the active theme. */
  swatch: [string, string];
}

/**
 * ThemeOption is the palette catalogue. Values map 1:1 to the token blocks
 * in styles/app.css; next-themes applies the value as the class on <html>.
 */
const THEME_OPTIONS: ThemeOption[] = [
  {
    value: "system",
    label: "System",
    hint: "Follow the OS setting",
    icon: Monitor,
    swatch: ["#ffffff", "#18181b"],
  },
  {
    value: "light",
    label: "Light",
    hint: "Default light palette",
    icon: Sun,
    swatch: ["#ffffff", "#18181b"],
  },
  {
    value: "dark",
    label: "Dark",
    hint: "Default dark palette",
    icon: Moon,
    swatch: ["#0a0a0a", "#fafafa"],
  },
  {
    value: "vscode-light",
    label: "VS Code Light",
    hint: "VS Code Light Modern",
    icon: Palette,
    swatch: ["#ffffff", "#005fb8"],
  },
  {
    value: "vscode-dark",
    label: "VS Code Dark",
    hint: "VS Code Dark Modern",
    icon: Palette,
    swatch: ["#181818", "#0078d4"],
  },
  {
    value: "catppuccin-latte",
    label: "Cappuccino Light",
    hint: "Catppuccin Latte",
    icon: Palette,
    swatch: ["#eff1f5", "#8839ef"],
  },
  {
    value: "catppuccin-mocha",
    label: "Cappuccino Dark",
    hint: "Catppuccin Mocha",
    icon: Palette,
    swatch: ["#1e1e2e", "#cba6f7"],
  },
];

const DARK_THEMES = new Set(["dark", "vscode-dark", "catppuccin-mocha"]);

/**
 * ThemeMenu replaces the plain light/dark toggle: every palette the app
 * ships is one click away, with a swatch preview next to each entry.
 */
export function ThemeMenu() {
  const { resolvedTheme, setTheme } = useTheme();
  const isDark = resolvedTheme !== undefined && DARK_THEMES.has(resolvedTheme);
  const TriggerIcon = resolvedTheme === undefined ? Palette : isDark ? Moon : Sun;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Pick a theme">
          <TriggerIcon className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>Theme</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {THEME_OPTIONS.map((option) => (
          <DropdownMenuItem key={option.value} onSelect={() => setTheme(option.value)}>
            <span
              aria-hidden
              className="size-4 shrink-0 rounded-full border"
              style={{ backgroundColor: option.swatch[0] }}
            >
              <span
                className="mx-auto mt-[3px] block size-2 rounded-full"
                style={{ backgroundColor: option.swatch[1] }}
              />
            </span>
            <span className="flex flex-col leading-tight">
              <span>{option.label}</span>
              <span className="text-xs text-muted-foreground">{option.hint}</span>
            </span>
            {resolvedTheme === option.value && <Check className="ml-auto size-4" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
