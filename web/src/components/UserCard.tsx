'use client';

import Link from 'next/link';
import type { Profile } from '@/lib/types';

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

function truncatePubkey(pubkey: string): string {
  if (pubkey.length <= 12) return pubkey;
  return `${pubkey.slice(0, 6)}...${pubkey.slice(-4)}`;
}

export default function UserCard({ profile }: { profile: Profile }) {
  const displayName = profile.displayName || truncatePubkey(profile.author);

  return (
    <Link
      href={`/user/${profile.author}`}
      className="flex items-center gap-3 p-3 hover:bg-gray-900/50 transition-colors rounded-lg"
    >
      <div className="shrink-0 w-12 h-12 rounded-full bg-gray-700 flex items-center justify-center text-sm font-bold text-white">
        {getInitials(displayName)}
      </div>
      <div className="flex-1 min-w-0">
        <p className="font-semibold text-white truncate">{displayName}</p>
        <p className="text-sm text-gray-400 truncate">
          {truncatePubkey(profile.author)}
        </p>
        {profile.bio && (
          <p className="text-sm text-gray-300 mt-1 line-clamp-2">
            {profile.bio}
          </p>
        )}
      </div>
    </Link>
  );
}
