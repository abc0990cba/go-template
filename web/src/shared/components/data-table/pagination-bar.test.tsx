import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PaginationBar, type ModeTarget } from "./pagination-bar";

const user = userEvent.setup();

function baseTarget(overrides: Partial<ModeTarget> = {}): ModeTarget {
  return {
    mode: "offset",
    view: "paged",
    page: 1,
    hasMore: true,
    onFirstPage: vi.fn(),
    onPreviousPage: vi.fn(),
    onNextPage: vi.fn(),
    onModeChange: vi.fn(),
    onViewChange: vi.fn(),
    ...overrides,
  };
}

describe("PaginationBar", () => {
  it("renders page position and moves forward and back in the paged view", async () => {
    const target = baseTarget({ page: 2 });
    render(<PaginationBar pagination={target} />);

    expect(screen.getByText("Page")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();

    const previous = screen.getByRole("button", { name: "Previous" });
    const next = screen.getByRole("button", { name: "Next" });
    expect(previous).toBeEnabled();
    expect(next).toBeEnabled();
    await user.click(previous);
    await user.click(next);
    expect(target.onPreviousPage).toHaveBeenCalledOnce();
    expect(target.onNextPage).toHaveBeenCalledOnce();
  });

  it("disables Previous on the first page and Next at the end of the list", () => {
    render(<PaginationBar pagination={baseTarget({ page: 1, hasMore: false })} />);
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });

  it("switches to continuation controls outside offset mode", async () => {
    // "Start over" is disabled on page 1 (there is nothing to restart).
    const target = baseTarget({ mode: "keyset", page: 2 });
    render(<PaginationBar pagination={target} />);

    await user.click(screen.getByRole("button", { name: "Start over" }));
    expect(target.onFirstPage).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Next page" })).toBeEnabled();
  });

  it("changes the pagination strategy through the mode switcher", async () => {
    const target = baseTarget();
    render(<PaginationBar pagination={target} />);

    expect(screen.getByRole("button", { name: "Offset" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "Cursor" }));
    expect(target.onModeChange).toHaveBeenCalledWith("cursor");
  });

  it("swaps page controls for the load-more feed in the infinite view", async () => {
    const target = baseTarget({ view: "infinite", loadedCount: 25, onLoadMore: vi.fn() });
    render(<PaginationBar pagination={target} />);

    expect(screen.getByText(/25 rows loaded/)).toBeInTheDocument();
    const loadMore = screen.getByRole("button", { name: "Load more" });
    await user.click(loadMore);
    expect(target.onLoadMore).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
  });
});
