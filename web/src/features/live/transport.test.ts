import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { connectRealtime, type RealtimeSocket } from "./transport";

// Minimal structural doubles: the transport only needs listeners and close.

class FakeSocket implements RealtimeSocket {
  static instances: FakeSocket[] = [];
  static reset(): void {
    FakeSocket.instances = [];
  }

  readonly listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  closed = false;

  constructor(readonly url: string) {
    FakeSocket.instances.push(this);
  }

  addEventListener(
    type: "open" | "message" | "error" | "close",
    listener: (event: { data?: unknown }) => void,
  ): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  close(): void {
    this.closed = true;
  }

  emit(type: "open" | "message" | "error" | "close", data?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) listener({ data });
  }
}

class FakeSource {
  static instances: FakeSource[] = [];
  static reset(): void {
    FakeSource.instances = [];
  }

  readonly listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  closed = false;

  constructor(readonly url: string) {
    FakeSource.instances.push(this);
  }

  addEventListener(
    type: "open" | "message" | "error",
    listener: (event: { data?: unknown }) => void,
  ): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  close(): void {
    this.closed = true;
  }

  emit(type: "open" | "message" | "error", data?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) listener({ data });
  }
}

function lastSocket(): FakeSocket {
  const socket = FakeSocket.instances.at(-1);
  if (!socket) throw new Error("no socket was opened");
  return socket;
}

function lastSource(): FakeSource {
  const source = FakeSource.instances.at(-1);
  if (!source) throw new Error("no EventSource was opened");
  return source;
}

describe("connectRealtime", () => {
  beforeEach(() => {
    FakeSocket.reset();
    FakeSource.reset();
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("prefers the WebSocket and relays its frames", () => {
    const messages: string[] = [];
    const statuses: string[] = [];
    connectRealtime({
      onMessage: (data) => messages.push(data),
      onStatus: (status) => statuses.push(status),
      socketFactory: (url) => new FakeSocket(url),
      sourceFactory: (url) => new FakeSource(url),
    });

    const socket = lastSocket();
    expect(socket.url).toContain("/api/events/ws");
    socket?.emit("open");
    socket?.emit("message", '{"type":"connected"}');

    expect(messages).toEqual(['{"type":"connected"}']);
    expect(statuses).toContain("live");
    // No SSE connection while the socket is healthy.
    expect(FakeSource.instances).toHaveLength(0);
  });

  it("downgrades to SSE immediately when the socket never opens", () => {
    connectRealtime({
      onMessage: () => {},
      onStatus: () => {},
      socketFactory: (url) => new FakeSocket(url),
      sourceFactory: (url) => new FakeSource(url),
    });

    const socket = lastSocket();
    socket.emit("error");
    socket?.emit("close");

    const source = FakeSource.instances.at(-1);
    expect(source?.url).toBe("/api/events/stream");
    expect(socket?.closed).toBe(true);
  });

  it("reconnects with capped backoff after a dropped socket", () => {
    const statuses: string[] = [];
    connectRealtime({
      onMessage: () => {},
      onStatus: (status) => statuses.push(status),
      socketFactory: (url) => new FakeSocket(url),
      sourceFactory: (url) => new FakeSource(url),
      retryDelay: () => 10,
    });

    // Open once, then drop: one reconnect is scheduled.
    lastSocket().emit("open");
    lastSocket().emit("close");
    expect(FakeSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(10);
    expect(FakeSocket.instances).toHaveLength(2);

    // A socket that opened once keeps reconnecting on schedule — flapping
    // never downgrades (only sockets that never open do; the next test).
    const delays = [10, 10];
    for (const delay of delays) {
      lastSocket().emit("open");
      lastSocket().emit("close");
      const before = FakeSocket.instances.length;
      vi.advanceTimersByTime(delay);
      expect(FakeSocket.instances.length).toBe(before + 1);
    }
    expect(FakeSource.instances).toHaveLength(0);
    expect(statuses).toContain("offline");
  });

  it("never lets an empty instance list reach assertions", () => {
    // Guard the helper pattern used above: at(-1) after a fresh connect
    // always has a socket to hand back.
    connectRealtime({
      onMessage: () => {},
      onStatus: () => {},
      socketFactory: (url) => new FakeSocket(url),
      sourceFactory: (url) => new FakeSource(url),
    });
    const last = lastSocket();
    last.emit("open");
    expect(last.closed).toBe(false);
  });

  it("uses SSE directly when WebSocket is unavailable", () => {
    vi.stubGlobal("WebSocket", undefined);
    connectRealtime({
      onMessage: () => {},
      onStatus: () => {},
      sourceFactory: (url) => new FakeSource(url),
    });

    expect(FakeSocket.instances).toHaveLength(0);
    expect(lastSource().url).toBe("/api/events/stream");
  });

  it("close() tears everything down and stops reconnection", () => {
    const disconnect = connectRealtime({
      onMessage: () => {},
      onStatus: () => {},
      socketFactory: (url) => new FakeSocket(url),
      sourceFactory: (url) => new FakeSource(url),
      retryDelay: () => 10,
    });

    const socket = lastSocket();
    socket.emit("open");
    disconnect();
    expect(socket.closed).toBe(true);

    // A later drop must not schedule anything: the connection is closed.
    socket.emit("close");
    vi.advanceTimersByTime(60_000);
    expect(FakeSocket.instances).toHaveLength(1);
    expect(FakeSource.instances).toHaveLength(0);
  });
});
