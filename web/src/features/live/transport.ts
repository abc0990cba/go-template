export type ConnectionStatus = "connecting" | "live" | "offline";

// The realtime transport: WebSocket first (the backend's bidirectional
// sibling of the SSE stream — same JSON frames, native pings), falling back
// to Server-Sent Events when the socket cannot be established. Consumers
// see one thing only: string frames of {"type","payload"} JSON and a status.

/**
 * The slice of WebSocket the transport uses; structural so tests inject
 * doubles without depending on a DOM WebSocket.
 */
export interface RealtimeSocket {
  close(): void;
  addEventListener(
    type: "open" | "message" | "error" | "close",
    listener: (event: { data?: unknown }) => void,
  ): void;
}

/** The slice of EventSource the transport uses. */
export interface RealtimeSource {
  close(): void;
  addEventListener(
    type: "open" | "message" | "error",
    listener: (event: { data?: unknown }) => void,
  ): void;
}

export interface RealtimeHandlers {
  /** One wire frame: the JSON string {"type","payload"}. */
  onMessage(data: string): void;
  onStatus(status: ConnectionStatus): void;
}

export interface RealtimeOptions extends RealtimeHandlers {
  /** Defaults to the global WebSocket; tests inject doubles. */
  socketFactory?: (url: string) => RealtimeSocket;
  /** Defaults to the global EventSource; tests inject doubles. */
  sourceFactory?: (url: string) => RealtimeSource;
  /** Reconnect delay before retrying the socket; capped exponential. */
  retryDelay?: (attempt: number) => number;
}

const MAX_SOCKET_ATTEMPTS = 3;

function defaultRetryDelay(attempt: number): number {
  return Math.min(30_000, 1000 * 2 ** attempt);
}

function defaultSocketFactory(url: string): RealtimeSocket {
  return new WebSocket(url);
}

function defaultSourceFactory(url: string): RealtimeSource {
  return new EventSource(url);
}

/**
 * Opens the live event stream and returns a disconnect function (safe to
 * call more than once). WebSocket reconnects with capped backoff; after
 * three failed attempts — or immediately, when the socket never opened —
 * the stream downgrades to SSE for its lifetime (EventSource then
 * reconnects natively on its own).
 */
export function connectRealtime(options: RealtimeOptions): () => void {
  const {
    onMessage,
    onStatus,
    socketFactory = defaultSocketFactory,
    sourceFactory = defaultSourceFactory,
    retryDelay = defaultRetryDelay,
  } = options;

  const wsURL = `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/api/events/ws`;

  let closed = false;
  let socket: RealtimeSocket | null = null;
  let currentSource: RealtimeSource | null = null;
  let usingSSE = false;
  let attempt = 0;
  let timer: ReturnType<typeof setTimeout> | null = null;

  function teardownCurrent(): void {
    if (socket) {
      socket.close();
      socket = null;
    }
    if (currentSource) {
      currentSource.close();
      currentSource = null;
    }
  }

  function openSSE(): void {
    if (closed) return;
    usingSSE = true;
    onStatus("connecting");
    const source = sourceFactory("/api/events/stream");
    source.addEventListener("open", () => onStatus("live"));
    source.addEventListener("error", () => {
      // EventSource reconnects natively; surface the gap for the badge.
      onStatus("offline");
    });
    source.addEventListener("message", (event) => {
      if (typeof event.data === "string") onMessage(event.data);
    });
    // Keep a handle so close() reaches the connection.
    currentSource = source;
  }

  function openSocket(): void {
    if (closed) return;
    onStatus("connecting");
    let opened = false;
    const ws = socketFactory(wsURL);
    socket = ws;
    ws.addEventListener("open", () => {
      opened = true;
      attempt = 0;
      onStatus("live");
    });
    ws.addEventListener("message", (event) => {
      if (typeof event.data === "string") onMessage(event.data);
    });
    ws.addEventListener("close", () => {
      if (closed || usingSSE) return;
      onStatus("offline");
      scheduleReconnect(opened);
    });
    ws.addEventListener("error", () => {
      // close always follows error; the close handler owns recovery.
    });
  }

  function scheduleReconnect(hadOpened: boolean): void {
    if (closed || usingSSE) return;
    // A socket that never opened means the transport is unavailable (proxy,
    // older backend): downgrade to SSE right away instead of retrying.
    if (!hadOpened) {
      teardownCurrent();
      openSSE();
      return;
    }
    attempt += 1;
    if (attempt > MAX_SOCKET_ATTEMPTS) {
      teardownCurrent();
      openSSE();
      return;
    }
    timer = setTimeout(() => {
      if (!closed) openSocket();
    }, retryDelay(attempt));
  }

  // jsdom (and exotic browsers) may lack WebSocket entirely — but an
  // injected factory always wins over the availability check.
  if (socketFactory === defaultSocketFactory && typeof globalThis.WebSocket === "undefined") {
    openSSE();
  } else {
    openSocket();
  }

  return () => {
    if (closed) return;
    closed = true;
    if (timer) clearTimeout(timer);
    teardownCurrent();
  };
}
