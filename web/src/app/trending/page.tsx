'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { getTrending } from '@/lib/api';
import type { FeedEntry } from '@/lib/types';
import PostCard from '@/components/PostCard';

export default function TrendingPage() {
  const [tags, setTags] = useState<{ tag: string; count: number }[]>([]);
  const [posts, setPosts] = useState<FeedEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const data = await getTrending();
        setTags(data.tags ?? []);
        setPosts(data.posts ?? []);
      } catch {
        setTags([]);
        setPosts([]);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  return (
    <div>
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3">
        <h1 className="text-xl font-bold text-white">Trending</h1>
      </header>

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : (
        <>
          {/* Trending Tags */}
          <div className="border-b border-gray-800">
            <h2 className="px-4 pt-4 pb-2 text-lg font-bold text-white">
              Trending Hashtags
            </h2>
            {tags.length === 0 ? (
              <div className="px-4 py-6 text-gray-500 text-sm">
                No trending hashtags yet
              </div>
            ) : (
              <div className="divide-y divide-gray-800">
                {tags.map((item, index) => (
                  <Link
                    key={item.tag}
                    href={`/search?q=%23${item.tag}`}
                    className="flex items-center justify-between px-4 py-3 hover:bg-gray-900/50 transition-colors"
                  >
                    <div>
                      <p className="text-xs text-gray-500">
                        {index + 1} · Trending
                      </p>
                      <p className="text-white font-semibold">#{item.tag}</p>
                      <p className="text-xs text-gray-500">
                        {item.count} {item.count === 1 ? 'post' : 'posts'}
                      </p>
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </div>

          {/* Trending Posts */}
          <div>
            <h2 className="px-4 pt-4 pb-2 text-lg font-bold text-white">
              Popular Posts
            </h2>
            {posts.length === 0 ? (
              <div className="px-4 py-6 text-gray-500 text-sm">
                No trending posts yet
              </div>
            ) : (
              posts.map((entry) => (
                <PostCard key={entry.post.id} entry={entry} />
              ))
            )}
          </div>
        </>
      )}
    </div>
  );
}
