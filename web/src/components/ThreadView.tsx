'use client';

import PostCard from './PostCard';
import type { FeedEntry } from '@/lib/types';

export default function ThreadView({
  replies,
  depth = 0,
}: {
  replies: FeedEntry[];
  depth?: number;
}) {
  if (replies.length === 0) {
    return (
      <div className="text-center py-8 text-gray-500">
        <p>No replies yet</p>
      </div>
    );
  }

  return (
    <div className={depth > 0 ? 'border-l-2 border-gray-800 ml-4' : ''}>
      {replies.map((entry) => (
        <PostCard key={entry.post.id} entry={entry} />
      ))}
    </div>
  );
}
