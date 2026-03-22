'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { wsClient } from '@/lib/ws';
import type { WSEvent } from '@/lib/types';

export function useWebSocket() {
  const [isConnected, setIsConnected] = useState(() => wsClient.isConnected);
  const [lastEvent, setLastEvent] = useState<WSEvent | null>(null);
  const handlersRef = useRef<Map<string, Set<(event: WSEvent) => void>>>(
    new Map()
  );

  useEffect(() => {
    wsClient.connect();

    const unsubConnected = wsClient.on('ws_connected', () => {
      setIsConnected(true);
    });

    const unsubDisconnected = wsClient.on('ws_disconnected', () => {
      setIsConnected(false);
    });

    const unsubAll = wsClient.on('*', (event) => {
      setLastEvent(event);
      handlersRef.current.get(event.type)?.forEach((h) => h(event));
    });

    return () => {
      unsubConnected();
      unsubDisconnected();
      unsubAll();
    };
  }, []);

  const subscribe = useCallback(
    (type: string, handler: (event: WSEvent) => void) => {
      if (!handlersRef.current.has(type)) {
        handlersRef.current.set(type, new Set());
      }
      handlersRef.current.get(type)!.add(handler);

      return () => {
        handlersRef.current.get(type)?.delete(handler);
      };
    },
    []
  );

  return { isConnected, lastEvent, subscribe };
}
