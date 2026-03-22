'use client';

import { useState, useEffect, useCallback } from 'react';
import { getNodeStatus } from '@/lib/api';
import type { NodeStatus, WSEvent } from '@/lib/types';
import { useWebSocket } from './useWebSocket';

export function useNodeStatus() {
  const [status, setStatus] = useState<NodeStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const { subscribe } = useWebSocket();

  const load = useCallback(async () => {
    try {
      const data = await getNodeStatus();
      setStatus(data);
    } catch {
      // Node not available
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 30000);
    return () => clearInterval(interval);
  }, [load]);

  useEffect(() => {
    const unsub = subscribe('node_status', (event: WSEvent) => {
      setStatus(event.data as NodeStatus);
    });
    return unsub;
  }, [subscribe]);

  return { status, loading, refresh: load };
}
