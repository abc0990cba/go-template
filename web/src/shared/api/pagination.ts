/** The shared pagination vocabulary of every list endpoint. */
export type PaginationMode = "offset" | "cursor" | "keyset";

/**
 * How a table presents pages: classic controls (Prev/Next) or a continuous
 * feed that appends rows as the user scrolls. Independent of the strategy —
 * infinite scroll works over offset, cursor, and keyset alike.
 */
export type ViewMode = "paged" | "infinite";

export interface ListParams {
  mode: PaginationMode;
  limit: number;
  offset: number;
  after: string;
  q: string;
  status: string;
  type: string;
  user_id: string;
}

/** One page of a server-paginated collection. */
export interface Paged<T> {
  data: T[];
  next_cursor: string;
}

/** buildListQuery renders the shared list vocabulary into a URL query string. */
export function buildListQuery(params: Partial<ListParams>): string {
  const search = new URLSearchParams();
  search.set("mode", params.mode ?? "offset");
  search.set("limit", String(params.limit ?? 10));
  if (params.mode === "offset" && (params.offset ?? 0) > 0) {
    search.set("offset", String(params.offset));
  }
  if (params.mode !== "offset" && params.after) {
    search.set("after", params.after);
  }
  if (params.q) search.set("q", params.q);
  if (params.status) search.set("status", params.status);
  if (params.type) search.set("type", params.type);
  if (params.user_id) search.set("user_id", params.user_id);
  return search.toString();
}
