'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { getTrending } from '@/lib/api';
import type { FeedEntry } from '@/lib/types';

function truncatePubkey(pubkey: string): string {
  if (pubkey.length <= 12) return pubkey;
  return `${pubkey.slice(0, 6)}...${pubkey.slice(-4)}`;
}

export default function TrendingList() {
  const [tags, setTags] = useState<{ tag: string; count: number }[]>([]);
  const [posts, setPosts] = useState<FeedEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const data = await getTrending();
        setTags((data.tags ?? []).slice(0, 5));
        setPosts((data.posts ?? []).slice(0, 3));
      } catch {
        setTags([]);
        setPosts([]);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  if (loading) {
    return (
      <div className="rounded-xl bg-gray-900 border border-gray-800 p-4">
        <h3 className="text-sm font-semibold text-white mb-3">Trending</h3>
        <div className="flex items-center justify-center py-4">
          <div className="w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-xl bg-gray-900 border border-gray-800 overflow-hidden">
      {/* Trending Topics */}
      <div className="p-4 pb-2">
        <h3 className="text-sm font-semibold text-white">Trending Topics</h3>
      </div>

      {tags.length === 0 ? (
        <p className="px-4 pb-3 text-sm text-gray-500">
          No trending topics yet
        </p>
      ) : (
        <div className="pb-2">
          {tags.map((item) => (
            <Link
              key={item.tag}
              href={`/search?q=%23${item.tag}`}
              className="block px-4 py-2 hover:bg-gray-800/50 transition-colors"
            >
              <p className="text-sm font-semibold text-white">#{item.tag}</p>
              <p className="text-xs text-gray-500">
                {item.count} {item.count === 1 ? 'post' : 'posts'}
              </p>
            </Link>
          ))}
        </div>
      )}

      {/* Popular Posts */}
      {posts.length > 0 && (
        <>
          <div className="border-t border-gray-800 p-4 pb-2">
            <h3 className="text-sm font-semibold text-white">Popular Posts</h3>
          </div>
          <div className="pb-2">
            {posts.map((entry) => {
              const displayName =
                entry.authorName || truncatePubkey(entry.post.author);
              return (
                <Link
                  key={entry.post.id}
                  href={`/post/${entry.post.id}`}
                  className="block px-4 py-2 hover:bg-gray-800/50 transition-colors"
                >
                  <p className="text-xs text-gray-400">{displayName}</p>
                  <p className="text-sm text-white line-clamp-2">
                    {entry.post.content}
                  </p>
                  <p className="text-xs text-gray-500 mt-0.5">
                    {entry.likeCount} likes -- {entry.replyCount} replies
                  </p>
                </Link>
              );
            })}
          </div>
        </>
      )}

      {/* See more */}
      <Link
        href="/trending"
        className="block px-4 py-3 border-t border-gray-800 text-sm text-blue-500 hover:text-blue-400 hover:bg-gray-800/50 transition-colors"
      >
        See more
      </Link>
    </div>
  );
}
