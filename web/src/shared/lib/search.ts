import { z } from "zod";

/**
 * listSearch is the URL vocabulary shared by every list page: pagination
 * mode, page/cursor state, and filters all live in the query string, so any
 * table state is linkable, refreshable, and back-button friendly.
 */
export const listSearchSchema = z.object({
  mode: z.enum(["offset", "cursor", "keyset"]).catch("offset").default("offset"),
  view: z.enum(["paged", "infinite"]).catch("paged").default("paged"),
  page: z.coerce.number().int().min(1).catch(1).default(1),
  after: z.string().optional(),
  q: z.string().optional(),
  status: z.string().optional(),
  type: z.string().optional(),
});

export type ListSearch = z.infer<typeof listSearchSchema>;
