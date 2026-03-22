'use client';

import { useEffect, useState, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useProfile } from '@/hooks/useProfile';
import { getUserPosts, followUser, unfollowUser } from '@/lib/api';
import type { FeedEntry } from '@/lib/types';
import PostCard from '@/components/PostCard';

type TabType = 'posts' | 'replies' | 'media' | 'likes';

const TABS: { id: TabType; label: string }[] = [
  { id: 'posts', label: 'Posts' },
  { id: 'replies', label: 'Replies' },
  { id: 'media', label: 'Media' },
  { id: 'likes', label: 'Likes' },
];

function truncatePubkey(pubkey: string): string {
  if (pubkey.length <= 16) return pubkey;
  return `${pubkey.slice(0, 8)}...${pubkey.slice(-6)}`;
}

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

export default function UserProfilePage() {
  const params = useParams<{ pubkey: string }>();
  const router = useRouter();
  const { profile, loading: profileLoading } = useProfile(params.pubkey ?? null);
  const [activeTab, setActiveTab] = useState<TabType>('posts');
  const [entries, setEntries] = useState<FeedEntry[]>([]);
  const [loadingPosts, setLoadingPosts] = useState(false);
  const [isFollowing, setIsFollowing] = useState(false);

  const loadPosts = useCallback(async () => {
    if (!params.pubkey) return;
    setLoadingPosts(true);
    try {
      const data = await getUserPosts(params.pubkey, activeTab);
      setEntries(data.entries ?? []);
    } catch {
      setEntries([]);
    } finally {
      setLoadingPosts(false);
    }
  }, [params.pubkey, activeTab]);

  useEffect(() => {
    loadPosts();
  }, [loadPosts]);

  const handleFollow = useCallback(async () => {
    if (!params.pubkey) return;
    try {
      if (isFollowing) {
        await unfollowUser(params.pubkey);
      } else {
        await followUser(params.pubkey);
      }
      setIsFollowing((prev) => !prev);
    } catch {
      // Silently handle
    }
  }, [params.pubkey, isFollowing]);

  const displayName =
    profile?.displayName || truncatePubkey(params.pubkey ?? '');

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
        <div>
          <h1 className="text-xl font-bold text-white">{displayName}</h1>
        </div>
      </header>

      {/* Banner */}
      <div className="h-32 bg-gray-800" />

      {/* Profile info */}
      <div className="px-4 pb-4">
        {/* Avatar row */}
        <div className="flex items-end justify-between -mt-12 mb-4">
          <div className="w-24 h-24 rounded-full bg-gray-700 border-4 border-gray-950 flex items-center justify-center text-2xl font-bold text-white">
            {profileLoading ? '...' : getInitials(displayName)}
          </div>
          <button
            onClick={handleFollow}
            className={`px-5 py-1.5 rounded-full font-bold text-sm transition-colors ${
              isFollowing
                ? 'bg-transparent border border-gray-600 text-white hover:border-red-500 hover:text-red-500'
                : 'bg-white text-black hover:bg-gray-200'
            }`}
          >
            {isFollowing ? 'Following' : 'Follow'}
          </button>
        </div>

        {/* Name and bio */}
        <h2 className="text-xl font-bold text-white">{displayName}</h2>
        <p className="text-gray-400 text-sm">
          {truncatePubkey(params.pubkey ?? '')}
        </p>
        {profile?.bio && (
          <p className="text-white mt-2">{profile.bio}</p>
        )}
        {profile?.website && (
          <a
            href={profile.website}
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-500 hover:underline text-sm mt-1 inline-block"
          >
            {profile.website}
          </a>
        )}
      </div>

      {/* Tabs */}
      <div className="flex border-b border-gray-800">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`flex-1 py-3 text-center text-sm font-medium transition-colors relative ${
              activeTab === tab.id
                ? 'text-white'
                : 'text-gray-500 hover:text-gray-300'
            }`}
          >
            {tab.label}
            {activeTab === tab.id && (
              <div className="absolute bottom-0 left-1/2 -translate-x-1/2 w-12 h-1 bg-blue-500 rounded-full" />
            )}
          </button>
        ))}
      </div>

      {/* Posts list */}
      {loadingPosts ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : entries.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <p>No {activeTab} yet</p>
        </div>
      ) : (
        entries.map((entry) => (
          <PostCard key={entry.post.id} entry={entry} />
        ))
      )}
    </div>
  );
}
