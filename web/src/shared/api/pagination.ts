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

/** One page of a server-paginated collection. The cursor is absent on the
 * last page, exactly as the spec's *List schemas declare it. */
export interface Paged<T> {
  data: T[];
  next_cursor?: string;
}
