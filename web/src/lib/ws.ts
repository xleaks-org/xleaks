import type { WSEvent } from './types';

type EventHandler = (event: WSEvent) => void;

function getWSUrl(): string {
  if (process.env.NEXT_PUBLIC_WS_URL) return process.env.NEXT_PUBLIC_WS_URL;
  if (typeof window === 'undefined') return 'ws://localhost:7470/ws';
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}/ws`;
}
const WS_URL = getWSUrl();
const RECONNECT_INTERVAL = 3000;
const MAX_RECONNECT_INTERVAL = 30000;

class WebSocketClient {
  private ws: WebSocket | null = null;
  private listeners: Map<string, Set<EventHandler>> = new Map();
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private reconnectDelay = RECONNECT_INTERVAL;
  private intentionalClose = false;
  private _isConnected = false;

  get isConnected(): boolean {
    return this._isConnected;
  }

  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN) return;

    this.intentionalClose = false;

    try {
      this.ws = new WebSocket(WS_URL);

      this.ws.onopen = () => {
        this._isConnected = true;
        this.reconnectDelay = RECONNECT_INTERVAL;
        this.emit({ type: 'ws_connected', data: null });
      };

      this.ws.onmessage = (event) => {
        try {
          const parsed = JSON.parse(event.data) as WSEvent;
          this.emit(parsed);
        } catch {
          // Ignore malformed messages
        }
      };

      this.ws.onclose = () => {
        this._isConnected = false;
        this.emit({ type: 'ws_disconnected', data: null });
        if (!this.intentionalClose) {
          this.scheduleReconnect();
        }
      };

      this.ws.onerror = () => {
        this._isConnected = false;
      };
    } catch {
      this.scheduleReconnect();
    }
  }

  disconnect(): void {
    this.intentionalClose = true;
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this._isConnected = false;
  }

  on(type: string, handler: EventHandler): () => void {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }
    this.listeners.get(type)!.add(handler);

    return () => {
      this.listeners.get(type)?.delete(handler);
    };
  }

  private emit(event: WSEvent): void {
    // Notify type-specific listeners
    this.listeners.get(event.type)?.forEach((handler) => handler(event));
    // Notify wildcard listeners
    this.listeners.get('*')?.forEach((handler) => handler(event));
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimeout) return;

    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.reconnectDelay = Math.min(
        this.reconnectDelay * 1.5,
        MAX_RECONNECT_INTERVAL
      );
      this.connect();
    }, this.reconnectDelay);
  }
}

// Singleton instance
export const wsClient = new WebSocketClient();
