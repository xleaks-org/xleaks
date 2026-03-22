'use client';

import Link from 'next/link';
import { useState, useCallback } from 'react';
import type { FeedEntry } from '@/lib/types';
import { createReaction } from '@/lib/api';

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

function formatRelativeTime(timestamp: number): string {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  if (diff < 60) return `${Math.floor(diff)}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d`;
  return new Date(timestamp * 1000).toLocaleDateString();
}

function truncatePubkey(pubkey: string): string {
  if (pubkey.length <= 12) return pubkey;
  return `${pubkey.slice(0, 6)}...${pubkey.slice(-4)}`;
}

export default function PostCard({ entry }: { entry: FeedEntry }) {
  const { post, authorName, likeCount, replyCount, repostCount, isLiked, isReposted } = entry;
  const [liked, setLiked] = useState(isLiked);
  const [likes, setLikes] = useState(likeCount);
  const [reposted, setReposted] = useState(isReposted);
  const [reposts, setReposts] = useState(repostCount);

  const handleLike = useCallback(
    async (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      try {
        await createReaction({ target: post.id, reactionType: 'like' });
        setLiked((prev) => !prev);
        setLikes((prev) => (liked ? prev - 1 : prev + 1));
      } catch {
        // Silently handle
      }
    },
    [post.id, liked]
  );

  const handleRepost = useCallback(
    async (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      try {
        await createReaction({ target: post.id, reactionType: 'repost' });
        setReposted((prev) => !prev);
        setReposts((prev) => (reposted ? prev - 1 : prev + 1));
      } catch {
        // Silently handle
      }
    },
    [post.id, reposted]
  );

  const displayName = authorName || truncatePubkey(post.author);

  return (
    <Link
      href={`/post/${post.id}`}
      className="block border-b border-gray-800 px-4 py-3 hover:bg-gray-900/50 transition-colors"
    >
      <div className="flex gap-3">
        {/* Avatar */}
        <div className="shrink-0 w-10 h-10 rounded-full bg-gray-700 flex items-center justify-center text-sm font-bold text-white">
          {getInitials(displayName)}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {/* Header */}
          <div className="flex items-center gap-2 text-sm">
            <span className="font-semibold text-white truncate">
              {displayName}
            </span>
            <span className="text-gray-500 truncate">
              {truncatePubkey(post.author)}
            </span>
            <span className="text-gray-600">·</span>
            <span className="text-gray-500 shrink-0">
              {formatRelativeTime(post.timestamp)}
            </span>
          </div>

          {/* Reply indicator */}
          {post.replyTo && (
            <p className="text-sm text-gray-500 mt-0.5">
              Replying to a post
            </p>
          )}

          {/* Body */}
          <p className="text-white mt-1 whitespace-pre-wrap break-words">
            {post.content}
          </p>

          {/* Tags */}
          {post.tags.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-2">
              {post.tags.map((tag) => (
                <span
                  key={tag}
                  className="text-blue-400 text-sm hover:underline"
                >
                  #{tag}
                </span>
              ))}
            </div>
          )}

          {/* Action bar */}
          <div className="flex items-center gap-6 mt-3 text-gray-500">
            {/* Reply */}
            <button
              className="flex items-center gap-1.5 hover:text-blue-400 transition-colors group"
              onClick={(e) => e.stopPropagation()}
            >
              <svg className="w-4.5 h-4.5 group-hover:text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 20.25c4.97 0 9-3.694 9-8.25s-4.03-8.25-9-8.25S3 7.444 3 12c0 2.104.859 4.023 2.273 5.48.432.447.74 1.04.586 1.641a4.483 4.483 0 01-.923 1.785A5.969 5.969 0 006 21c1.282 0 2.47-.402 3.445-1.087.81.22 1.668.337 2.555.337z" />
              </svg>
              <span className="text-xs">{replyCount > 0 ? replyCount : ''}</span>
            </button>

            {/* Repost */}
            <button
              className={`flex items-center gap-1.5 transition-colors group ${
                reposted ? 'text-green-500' : 'hover:text-green-500'
              }`}
              onClick={handleRepost}
            >
              <svg className="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 12c0-1.232-.046-2.453-.138-3.662a4.006 4.006 0 00-3.7-3.7 48.678 48.678 0 00-7.324 0 4.006 4.006 0 00-3.7 3.7c-.017.22-.032.441-.046.662M19.5 12l3-3m-3 3l-3-3m-12 3c0 1.232.046 2.453.138 3.662a4.006 4.006 0 003.7 3.7 48.656 48.656 0 007.324 0 4.006 4.006 0 003.7-3.7c.017-.22.032-.441.046-.662M4.5 12l3 3m-3-3l-3 3" />
              </svg>
              <span className="text-xs">{reposts > 0 ? reposts : ''}</span>
            </button>

            {/* Like */}
            <button
              className={`flex items-center gap-1.5 transition-colors group ${
                liked ? 'text-pink-500' : 'hover:text-pink-500'
              }`}
              onClick={handleLike}
            >
              <svg
                className="w-4.5 h-4.5"
                viewBox="0 0 24 24"
                fill={liked ? 'currentColor' : 'none'}
                stroke="currentColor"
                strokeWidth={1.5}
              >
                <path strokeLinecap="round" strokeLinejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
              </svg>
              <span className="text-xs">{likes > 0 ? likes : ''}</span>
            </button>

            {/* Share */}
            <button
              className="flex items-center gap-1.5 hover:text-blue-400 transition-colors"
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                navigator.clipboard?.writeText(
                  `${window.location.origin}/post/${post.id}`
                );
              }}
            >
              <svg className="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </Link>
  );
}
