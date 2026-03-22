'use client';

import { useState, useCallback } from 'react';
import { searchPosts, searchUsers } from '@/lib/api';
import type { FeedEntry, Profile } from '@/lib/types';
import PostCard from '@/components/PostCard';
import UserCard from '@/components/UserCard';

type SearchTab = 'posts' | 'users';

export default function SearchPage() {
  const [query, setQuery] = useState('');
  const [activeTab, setActiveTab] = useState<SearchTab>('posts');
  const [posts, setPosts] = useState<FeedEntry[]>([]);
  const [users, setUsers] = useState<Profile[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  const handleSearch = useCallback(
    async (e?: React.FormEvent) => {
      e?.preventDefault();
      if (!query.trim()) return;
      setLoading(true);
      setSearched(true);
      try {
        if (activeTab === 'posts') {
          const data = await searchPosts(query);
          setPosts(data.entries ?? []);
        } else {
          const data = await searchUsers(query);
          setUsers(data.profiles ?? []);
        }
      } catch {
        setPosts([]);
        setUsers([]);
      } finally {
        setLoading(false);
      }
    },
    [query, activeTab]
  );

  return (
    <div>
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3">
        <h1 className="text-xl font-bold text-white mb-3">Search</h1>

        {/* Search input */}
        <form onSubmit={handleSearch}>
          <div className="relative">
            <svg
              className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-500"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search posts and users..."
              className="w-full bg-gray-800 border border-gray-700 rounded-full pl-10 pr-4 py-2 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors"
            />
          </div>
        </form>
      </header>

      {/* Tabs */}
      <div className="flex border-b border-gray-800">
        <button
          onClick={() => setActiveTab('posts')}
          className={`flex-1 py-3 text-center text-sm font-medium transition-colors relative ${
            activeTab === 'posts'
              ? 'text-white'
              : 'text-gray-500 hover:text-gray-300'
          }`}
        >
          Posts
          {activeTab === 'posts' && (
            <div className="absolute bottom-0 left-1/2 -translate-x-1/2 w-12 h-1 bg-blue-500 rounded-full" />
          )}
        </button>
        <button
          onClick={() => setActiveTab('users')}
          className={`flex-1 py-3 text-center text-sm font-medium transition-colors relative ${
            activeTab === 'users'
              ? 'text-white'
              : 'text-gray-500 hover:text-gray-300'
          }`}
        >
          Users
          {activeTab === 'users' && (
            <div className="absolute bottom-0 left-1/2 -translate-x-1/2 w-12 h-1 bg-blue-500 rounded-full" />
          )}
        </button>
      </div>

      {/* Results */}
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : !searched ? (
        <div className="text-center py-12 text-gray-500">
          <svg className="w-12 h-12 mx-auto mb-3 text-gray-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <p className="text-lg">Search XLeaks</p>
          <p className="text-sm mt-1">
            Find posts, users, and hashtags
          </p>
        </div>
      ) : activeTab === 'posts' ? (
        posts.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <p>No posts found for &quot;{query}&quot;</p>
          </div>
        ) : (
          posts.map((entry) => (
            <PostCard key={entry.post.id} entry={entry} />
          ))
        )
      ) : users.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <p>No users found for &quot;{query}&quot;</p>
        </div>
      ) : (
        <div className="p-2">
          {users.map((profile) => (
            <UserCard key={profile.author} profile={profile} />
          ))}
        </div>
      )}
    </div>
  );
}
