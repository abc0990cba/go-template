/**
 * A controllable EventSource stand-in for jsdom, which lacks the API
 * entirely. LiveProvider opens one SSE connection on mount; tests grab the
 * newest instance and push frames through message()/open()/error() to
 * exercise the live-update paths without a backend.
 */
export class MockEventSource {
  static instances: MockEventSource[] = [];

  /** The connection the component under test just opened. */
  static get last(): MockEventSource {
    const instance = this.instances.at(-1);
    if (!instance) throw new Error("no EventSource was opened");
    return instance;
  }

  static reset(): void {
    this.instances = [];
  }

  readonly url: string;
  readyState = 0; // 0 connecting, 1 open, 2 closed
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;

  private listeners = new Map<string, Array<(event: Event) => void>>();

  constructor(url: string | URL) {
    this.url = String(url);
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: Event) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  removeEventListener(type: string, listener: (event: Event) => void): void {
    this.listeners.set(
      type,
      (this.listeners.get(type) ?? []).filter((candidate) => candidate !== listener),
    );
  }

  close(): void {
    this.readyState = 2;
  }

  // --- test controls (not part of the real API) ---

  private dispatch(type: string, event: Event): void {
    for (const listener of this.listeners.get(type) ?? []) listener(event);
    if (type === "open" && this.onopen) this.onopen(event);
    if (type === "error" && this.onerror) this.onerror(event);
    if (type === "message" && this.onmessage) this.onmessage(event as MessageEvent);
  }

  /** The server accepted the connection. */
  open(): void {
    this.readyState = 1;
    this.dispatch("open", new Event("open"));
  }

  /** The connection dropped (EventSource would reconnect on its own). */
  fail(): void {
    this.readyState = 0;
    this.dispatch("error", new Event("error"));
  }

  /** A `data:` frame; `payload` is what the backend JSON-encodes. */
  message(payload: unknown): void {
    const event = new MessageEvent("message", { data: JSON.stringify(payload) });
    this.dispatch("message", event);
  }
}
