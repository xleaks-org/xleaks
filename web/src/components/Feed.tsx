'use client';

import { useEffect, useRef, useCallback } from 'react';
import PostCard from './PostCard';
import { useFeed } from '@/hooks/useFeed';

export default function Feed() {
  const {
    entries,
    loading,
    loadingMore,
    hasMore,
    loadMore,
    newPostsCount,
    showNewPosts,
  } = useFeed();
  const observerRef = useRef<HTMLDivElement>(null);

  const handleIntersection = useCallback(
    (observerEntries: IntersectionObserverEntry[]) => {
      if (observerEntries[0]?.isIntersecting && hasMore && !loadingMore) {
        loadMore();
      }
    },
    [hasMore, loadingMore, loadMore]
  );

  useEffect(() => {
    const node = observerRef.current;
    if (!node) return;

    const observer = new IntersectionObserver(handleIntersection, {
      rootMargin: '200px',
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [handleIntersection]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  return (
    <div>
      {/* New posts banner */}
      {newPostsCount > 0 && (
        <button
          onClick={showNewPosts}
          className="w-full py-3 text-center text-blue-500 bg-gray-900/80 hover:bg-gray-900 border-b border-gray-800 transition-colors text-sm font-medium"
        >
          Show {newPostsCount} new {newPostsCount === 1 ? 'post' : 'posts'}
        </button>
      )}

      {/* Posts */}
      {entries.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <p className="text-lg">No posts yet</p>
          <p className="text-sm mt-1">Be the first to share something</p>
        </div>
      ) : (
        entries.map((entry) => (
          <PostCard key={entry.post.id} entry={entry} />
        ))
      )}

      {/* Infinite scroll trigger */}
      <div ref={observerRef} className="h-1" />

      {/* Loading more */}
      {loadingMore && (
        <div className="flex items-center justify-center py-6">
          <div className="w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
          <span className="ml-2 text-gray-400 text-sm">Loading more...</span>
        </div>
      )}
    </div>
  );
}
