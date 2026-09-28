import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

/**
 * Deterministic gradient pairs (Tailwind v4 linear-gradient utilities). The
 * palette is a const literal so Tailwind's scanner sees every class; the
 * pick is a hash of the email, so the same person always wears the same
 * color across every table, card, and chat-like list in the app.
 */
const GRADIENTS = [
  "from-rose-500 to-orange-400",
  "from-amber-500 to-yellow-400",
  "from-emerald-500 to-teal-400",
  "from-sky-500 to-indigo-400",
  "from-violet-500 to-fuchsia-400",
  "from-cyan-500 to-blue-500",
  "from-pink-500 to-rose-500",
  "from-lime-500 to-emerald-500",
] as const;

/** FNV-1a keeps the pick stable without shipping a hash library. */
function hashSeed(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

/** First letters of the name parts (or email), uppercase — "Ada L". */
export function userInitials(nameOrEmail: string) {
  const parts = nameOrEmail.split(/[\s@.]+/).filter(Boolean);
  const first = parts.at(0)?.charAt(0) ?? "?";
  const second = parts.at(1)?.charAt(0) ?? "";
  return (first + second).toUpperCase();
}

/**
 * UserAvatar is the app-wide user face: initials over a per-user gradient.
 * Used everywhere a person shows up — users table, user card hero, org
 * seats, audit timeline, demo account picker — so a user is recognizable
 * by color at a glance.
 */
export function UserAvatar({
  name,
  email,
  className,
  fallbackClassName,
}: {
  name?: string;
  email?: string;
  className?: string;
  fallbackClassName?: string;
}) {
  const seed = (email || name || "?").trim().toLowerCase();
  const gradient = GRADIENTS[hashSeed(seed) % GRADIENTS.length];
  return (
    <Avatar className={className}>
      <AvatarFallback
        className={cn(
          "bg-linear-to-br font-semibold text-white select-none",
          gradient,
          fallbackClassName,
        )}
      >
        {userInitials(name || email || "?")}
      </AvatarFallback>
    </Avatar>
  );
}
