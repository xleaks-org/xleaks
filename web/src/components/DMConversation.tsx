'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { getConversation, sendDM } from '@/lib/api';
import type { DirectMessage } from '@/lib/types';

function formatTime(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  });
}

function truncatePubkey(pubkey: string): string {
  if (pubkey.length <= 12) return pubkey;
  return `${pubkey.slice(0, 6)}...${pubkey.slice(-4)}`;
}

export default function DMConversation({ pubkey }: { pubkey: string }) {
  const [messages, setMessages] = useState<DirectMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const loadMessages = useCallback(async () => {
    try {
      const data = await getConversation(pubkey);
      setMessages(data ?? []);
    } catch {
      setMessages([]);
    } finally {
      setLoading(false);
    }
  }, [pubkey]);

  useEffect(() => {
    loadMessages();
  }, [loadMessages]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = useCallback(async () => {
    if (!input.trim() || sending) return;
    setSending(true);
    try {
      await sendDM({ recipient: pubkey, content: input.trim() });
      setInput('');
      await loadMessages();
    } catch {
      // Handle error silently
    } finally {
      setSending(false);
    }
  }, [input, pubkey, sending, loadMessages]);

  return (
    <div className="flex flex-col h-full">
      {/* E2E notice */}
      <div className="px-4 py-2 bg-gray-900/50 border-b border-gray-800 text-center">
        <p className="text-xs text-gray-500 flex items-center justify-center gap-1.5">
          <svg
            className="w-3 h-3"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z"
            />
          </svg>
          Messages are end-to-end encrypted
        </p>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : messages.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <p>No messages yet</p>
            <p className="text-sm mt-1">
              Say hello to {truncatePubkey(pubkey)}
            </p>
          </div>
        ) : (
          messages.map((msg) => {
            const isSent = msg.author !== pubkey;
            return (
              <div
                key={msg.id}
                className={`flex ${isSent ? 'justify-end' : 'justify-start'}`}
              >
                <div
                  className={`max-w-[70%] rounded-2xl px-4 py-2 ${
                    isSent
                      ? 'bg-blue-500 text-white'
                      : 'bg-gray-800 text-white'
                  }`}
                >
                  <p className="text-sm break-words">{msg.id}</p>
                  <p
                    className={`text-xs mt-1 ${
                      isSent ? 'text-blue-100' : 'text-gray-500'
                    }`}
                  >
                    {formatTime(msg.timestamp)}
                  </p>
                </div>
              </div>
            );
          })
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-gray-800 p-4 shrink-0">
        <div className="flex items-center gap-3">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Type a message..."
            className="flex-1 bg-gray-800 border border-gray-700 rounded-full px-4 py-2 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors"
          />
          <button
            onClick={handleSend}
            disabled={!input.trim() || sending}
            className="bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white rounded-full p-2.5 transition-colors"
            aria-label="Send message"
          >
            <svg
              className="w-5 h-5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5"
              />
            </svg>
          </button>
        </div>
      </div>
    </div>
  );
}
