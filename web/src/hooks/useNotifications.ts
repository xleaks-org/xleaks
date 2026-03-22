'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  getNotifications,
  markNotificationsRead,
  getUnreadCount,
} from '@/lib/api';
import type { Notification } from '@/lib/types';
import { useWebSocket } from './useWebSocket';

export function useNotifications() {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const { subscribe } = useWebSocket();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [notifs, unread] = await Promise.all([
        getNotifications(),
        getUnreadCount(),
      ]);
      setNotifications(notifs ?? []);
      setUnreadCount(unread?.count ?? 0);
    } catch {
      // API not available
    } finally {
      setLoading(false);
    }
  }, []);

  const markRead = useCallback(async () => {
    try {
      await markNotificationsRead();
      setUnreadCount(0);
      setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
    } catch {
      // Silently handle
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const unsub = subscribe('new_notification', () => {
      setUnreadCount((c) => c + 1);
      load();
    });
    return unsub;
  }, [subscribe, load]);

  return { notifications, unreadCount, loading, markRead, refresh: load };
}
