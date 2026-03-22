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
      const raw = await getNodeStatus();
      // Normalize API response to match expected interface
      const normalized: NodeStatus = {
        peers: raw.peers ?? 0,
        bandwidth: {
          totalIn: raw.bandwidth?.totalIn ?? raw.bandwidth?.total_in ?? 0,
          totalOut: raw.bandwidth?.totalOut ?? raw.bandwidth?.total_out ?? 0,
        },
        storage: {
          usedGB: raw.storage?.usedGB ?? (raw.storage?.used ?? 0) / (1024 * 1024 * 1024),
          maxGB: raw.storage?.maxGB ?? (raw.storage?.limit ?? 0) / (1024 * 1024 * 1024),
        },
        uptime: raw.uptime ?? 0,
      };
      setStatus(normalized);
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
