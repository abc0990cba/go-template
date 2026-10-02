import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { listTickets } from "@/features/tickets";
import { infiniteRows, useDebounced, usePagedList, useTableView } from "./use-paged-list";
import type { Paged } from "@/shared/api/pagination";
import type { Ticket } from "@/features/tickets";
import { page, ticketFixture } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";

/** A fresh QueryClient per hook render — the shared one would leak caches. */
function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

describe("usePagedList", () => {
  it("requests the shared list vocabulary and returns the page", async () => {
    const seen: string[] = [];
    server.use(
      http.get("/api/tickets", ({ request }) => {
        seen.push(new URL(request.url).search);
        return HttpResponse.json(page([ticketFixture({ title: "First ticket" })]));
      }),
    );

    const { result } = renderHook(
      () =>
        usePagedList<Ticket>("tickets", listTickets, {
          mode: "offset",
          limit: 10,
          scope: "my",
        }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.data?.data).toHaveLength(1));
    expect(result.current.data?.data[0]?.title).toBe("First ticket");
    expect(seen[0]).toContain("mode=offset");
    expect(seen[0]).toContain("limit=10");
    expect(seen[0]).toContain("scope=my");
  });

  it("does not hit the network while disabled", async () => {
    const seen: string[] = [];
    server.use(
      http.get("/api/tickets", ({ request }) => {
        seen.push(new URL(request.url).search);
        return HttpResponse.json(page([]));
      }),
    );

    const { result } = renderHook(
      () =>
        usePagedList<Ticket>(
          "tickets",
          listTickets,
          { mode: "offset", limit: 10 },
          { enabled: false },
        ),
      { wrapper: makeWrapper() },
    );

    expect(result.current.isPending).toBe(true);
    expect(seen).toHaveLength(0);
  });
});

describe("useTableView (paged)", () => {
  it("reports rows, pagination position, and hasMore from next_cursor", async () => {
    server.use(
      http.get("/api/tickets", () =>
        HttpResponse.json(
          page([ticketFixture({ id: "t-1" }), ticketFixture({ id: "t-2" })], "cursor-2"),
        ),
      ),
    );

    const { result } = renderHook(
      () => useTableView<Ticket>("tickets", listTickets, { mode: "offset", limit: 10 }, "paged"),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.rows).toHaveLength(2));
    expect(result.current.hasMore).toBe(true);
    expect(result.current.nextCursor).toBe("cursor-2");
  });

  it("marks the end of the list with an empty next_cursor", async () => {
    server.use(http.get("/api/tickets", () => HttpResponse.json(page([ticketFixture()]))));

    const { result } = renderHook(
      () => useTableView<Ticket>("tickets", listTickets, { mode: "offset", limit: 10 }, "paged"),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.rows).toHaveLength(1));
    expect(result.current.hasMore).toBe(false);
  });
});

describe("useTableView (infinite)", () => {
  it("appends the next page when loadMore is called", async () => {
    const offsets: number[] = [];
    server.use(
      http.get("/api/tickets", ({ request }) => {
        const search = new URL(request.url).searchParams;
        // buildListQuery omits offset=0, the feed's starting position.
        offsets.push(Number(search.get("offset") ?? 0));
        const offset = Number(search.get("offset") ?? 0);
        return offset === 0
          ? HttpResponse.json(
              page([ticketFixture({ id: "t-1" }), ticketFixture({ id: "t-2" })], "more"),
            )
          : HttpResponse.json(page([ticketFixture({ id: "t-3" })]));
      }),
    );

    const { result } = renderHook(
      () => useTableView<Ticket>("tickets", listTickets, { mode: "offset", limit: 2 }, "infinite"),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.rows).toHaveLength(2));
    act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.rows).toHaveLength(3));
    expect(result.current.rows.map((ticket) => ticket.id)).toEqual(["t-1", "t-2", "t-3"]);
    expect(result.current.hasMore).toBe(false);
    expect(offsets).toEqual([0, 2]);
  });
});

describe("infiniteRows", () => {
  it("flattens accumulated pages and tolerates null page data", () => {
    // A nil slice on the wire arrives as null.
    const pages: Paged<Ticket>[] = [
      { data: [ticketFixture({ id: "t-1" })], next_cursor: "x" },
      { data: null as unknown as Ticket[], next_cursor: "" },
    ];
    expect(infiniteRows({ data: { pages } }).map((ticket) => ticket.id)).toEqual(["t-1"]);
    expect(infiniteRows({})).toEqual([]);
  });
});

describe("useDebounced", () => {
  it("delays rapid value changes", () => {
    vi.useFakeTimers();
    try {
      const { result, rerender } = renderHook(({ value }) => useDebounced(value, 300), {
        initialProps: { value: "a" },
        wrapper: makeWrapper(),
      });
      rerender({ value: "ab" });
      rerender({ value: "abc" });
      expect(result.current).toBe("a");
      act(() => {
        vi.advanceTimersByTime(300);
      });
      expect(result.current).toBe("abc");
    } finally {
      vi.useRealTimers();
    }
  });
});
