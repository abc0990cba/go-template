import { useSuspenseQuery } from "@tanstack/react-query";
import { meQuery } from "@/shared/lib/auth";
import type { Me } from "@/lib/types";

/**
 * useCan returns a predicate over the caller's expanded permissions
 * ("users:read", "jobs:execute", …). The backend resolves wildcards, so
 * the check is a plain set lookup.
 */
export function useCan(): (object: string, action: string) => boolean {
  const { data: me } = useSuspenseQuery(meQuery());
  const set = new Set(me.permissions);
  return (object, action) => set.has(`${object}:${action}`);
}

/** useMe is the shorthand for the session probe data. */
export function useMe(): Me {
  const { data } = useSuspenseQuery(meQuery());
  return data;
}
