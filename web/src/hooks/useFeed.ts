'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { getFeed } from '@/lib/api';
import type { FeedEntry } from '@/lib/types';
import { useWebSocket } from './useWebSocket';

export function useFeed() {
  const [entries, setEntries] = useState<FeedEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [newPostsCount, setNewPostsCount] = useState(0);
  const cursorRef = useRef<string | undefined>(undefined);
  const { subscribe } = useWebSocket();

  const loadFeed = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getFeed(undefined, 20);
      setEntries(data.entries ?? []);
      cursorRef.current = data.nextCursor;
      setHasMore(!!data.nextCursor);
    } catch {
      // API not available yet - that's fine
      setEntries([]);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadMore = useCallback(async () => {
    if (loadingMore || !hasMore) return;
    setLoadingMore(true);
    try {
      const data = await getFeed(cursorRef.current, 20);
      setEntries((prev) => [...prev, ...(data.entries ?? [])]);
      cursorRef.current = data.nextCursor;
      setHasMore(!!data.nextCursor);
    } catch {
      // Silently handle
    } finally {
      setLoadingMore(false);
    }
  }, [loadingMore, hasMore]);

  const showNewPosts = useCallback(() => {
    setNewPostsCount(0);
    loadFeed();
  }, [loadFeed]);

  useEffect(() => {
    loadFeed();
  }, [loadFeed]);

  useEffect(() => {
    const unsub = subscribe('new_post', () => {
      setNewPostsCount((c) => c + 1);
    });
    return unsub;
  }, [subscribe]);

  return {
    entries,
    loading,
    loadingMore,
    hasMore,
    loadMore,
    newPostsCount,
    showNewPosts,
    refresh: loadFeed,
  };
}
