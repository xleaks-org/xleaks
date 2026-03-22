'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { getConversations } from '@/lib/api';
import type { ConversationSummary } from '@/lib/types';
import { getInitials, truncatePubkey } from '@/lib/formatters';

function formatTime(timestamp: number): string {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  if (diff < 60) return 'now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
  return new Date(timestamp * 1000).toLocaleDateString();
}

export default function MessagesPage() {
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const data = await getConversations();
        setConversations(data ?? []);
      } catch {
        setConversations([]);
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
        <h1 className="text-xl font-bold text-white">Messages</h1>
      </header>

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : conversations.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <svg className="w-12 h-12 mx-auto mb-3 text-gray-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
          </svg>
          <p className="text-lg">No messages</p>
          <p className="text-sm mt-1">
            Direct messages are end-to-end encrypted
          </p>
        </div>
      ) : (
        <div>
          {conversations.map((conv) => {
            const name = conv.displayName || truncatePubkey(conv.pubkey);
            return (
              <Link
                key={conv.pubkey}
                href={`/messages/${conv.pubkey}`}
                className="flex items-center gap-3 px-4 py-3 border-b border-gray-800 hover:bg-gray-900/50 transition-colors"
              >
                <div className="shrink-0 w-12 h-12 rounded-full bg-gray-700 flex items-center justify-center text-sm font-bold text-white">
                  {getInitials(name)}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-white truncate">
                      {name}
                    </span>
                    <span className="text-xs text-gray-500 shrink-0 ml-2">
                      {formatTime(conv.timestamp)}
                    </span>
                  </div>
                  <p className="text-sm text-gray-400 truncate">
                    {conv.lastMessage}
                  </p>
                </div>
                {conv.unread > 0 && (
                  <span className="bg-blue-500 text-white text-xs font-bold rounded-full w-5 h-5 flex items-center justify-center shrink-0">
                    {conv.unread}
                  </span>
                )}
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
