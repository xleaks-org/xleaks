'use client';

import { useParams, useRouter } from 'next/navigation';
import DMConversation from '@/components/DMConversation';
import { truncatePubkey } from '@/lib/formatters';

export default function ConversationPage() {
  const params = useParams<{ pubkey: string }>();
  const router = useRouter();

  return (
    <div className="flex flex-col h-screen">
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3 flex items-center gap-4 shrink-0">
        <button
          onClick={() => router.back()}
          className="text-white hover:text-gray-300 transition-colors"
          aria-label="Go back"
        >
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" />
          </svg>
        </button>
        <h1 className="text-lg font-bold text-white truncate">
          {truncatePubkey(params.pubkey ?? '')}
        </h1>
      </header>

      {/* Conversation */}
      <div className="flex-1 min-h-0">
        <DMConversation pubkey={params.pubkey ?? ''} />
      </div>
    </div>
  );
}
