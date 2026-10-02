import { z } from "zod";
import { listSearchSchema } from "@/shared/lib/search";

/** URL vocabulary of the tickets table: the shared list state plus the
 * requester's scope tab. */
export const ticketsSearchSchema = listSearchSchema.extend({
  scope: z.enum(["my", "all"]).catch("my").default("my"),
});

export type TicketsSearch = z.infer<typeof ticketsSearchSchema>;
