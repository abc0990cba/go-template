import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, vi } from "vitest";
import { queryClient } from "@/shared/lib/query-client";
import { MockEventSource } from "./event-source";
import { server } from "./mocks/server";

// --- network: every fetch goes through the MSW handler list ---
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

afterEach(() => {
  cleanup();
  queryClient.clear();
  localStorage.clear();
  MockEventSource.reset();
});

// --- jsdom gaps the app relies on ---

// LiveProvider opens a Server-Sent Events connection on mount; jsdom has no
// EventSource, so the app would crash before rendering anything.
vi.stubGlobal("EventSource", MockEventSource);
// jsdom has a WebSocket, but the realtime transport must default to the
// SSE stub in component tests; transport tests inject their own doubles.
vi.stubGlobal("WebSocket", undefined);

// Radix components and Tailwind's media-query hooks need matchMedia.
if (typeof window.matchMedia !== "function") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

// Radix Popper positions with ResizeObserver; jsdom lacks it.
if (typeof globalThis.ResizeObserver === "undefined") {
  class ResizeObserverMock {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal("ResizeObserver", ResizeObserverMock);
}

// Radix Select scrolls the highlighted option into view.
Element.prototype.scrollIntoView ??= () => {};

// Radix Select and DropdownMenu manage focus through pointer capture,
// which jsdom does not implement; without the stubs their popovers never open.
Element.prototype.hasPointerCapture ??= () => false;
Element.prototype.setPointerCapture ??= () => {};
Element.prototype.releasePointerCapture ??= () => {};
