'use client';

import { useEffect, useState, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { getThread } from '@/lib/api';
import type { FeedEntry } from '@/lib/types';
import PostCard from '@/components/PostCard';
import PostComposer from '@/components/PostComposer';
import ThreadView from '@/components/ThreadView';

export default function PostDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const [root, setRoot] = useState<FeedEntry | null>(null);
  const [replies, setReplies] = useState<FeedEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadThread = useCallback(async () => {
    if (!params.id) return;
    setLoading(true);
    setError(null);
    try {
      const data = await getThread(params.id);
      setRoot(data.root);
      setReplies(data.replies ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load post');
    } finally {
      setLoading(false);
    }
  }, [params.id]);

  useEffect(() => {
    loadThread();
  }, [loadThread]);

  return (
    <div>
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3 flex items-center gap-4">
        <button
          onClick={() => router.back()}
          className="text-white hover:text-gray-300 transition-colors"
          aria-label="Go back"
        >
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" />
          </svg>
        </button>
        <h1 className="text-xl font-bold text-white">Post</h1>
      </header>

      {loading && (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      )}

      {error && (
        <div className="text-center py-12">
          <p className="text-red-400">{error}</p>
          <button
            onClick={loadThread}
            className="mt-3 text-blue-500 hover:text-blue-400"
          >
            Try again
          </button>
        </div>
      )}

      {root && (
        <>
          {/* Main post */}
          <PostCard entry={root} />

          {/* Reply composer */}
          <PostComposer replyTo={root.post.id} onPostCreated={loadThread} />

          {/* Replies */}
          <div className="border-t border-gray-800">
            <h2 className="px-4 py-3 text-lg font-bold text-white">Replies</h2>
            <ThreadView replies={replies} />
          </div>
        </>
      )}
    </div>
  );
}
