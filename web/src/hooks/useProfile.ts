'use client';

import { useState, useEffect, useCallback } from 'react';
import { getProfile } from '@/lib/api';
import type { Profile } from '@/lib/types';

export function useProfile(pubkey: string | null) {
  const [profile, setProfile] = useState<Profile | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!pubkey) return;
    setLoading(true);
    setError(null);
    try {
      const data = await getProfile(pubkey);
      setProfile(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load profile');
    } finally {
      setLoading(false);
    }
  }, [pubkey]);

  useEffect(() => {
    load();
  }, [load]);

  return { profile, loading, error, refresh: load };
}
