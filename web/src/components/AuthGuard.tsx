'use client';

import { useEffect, useState, useCallback } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { getActiveIdentity, unlockIdentity } from '@/lib/api';
import PassphrasePrompt from './PassphrasePrompt';

const PUBLIC_PATHS = ['/onboarding'];

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [state, setState] = useState<'loading' | 'ready' | 'onboarding' | 'locked'>('loading');

  const checkIdentity = useCallback(async () => {
    try {
      const identity = await getActiveIdentity();
      if (!identity || identity.needsOnboarding) {
        setState('onboarding');
        return;
      }
      if (identity.locked) {
        setState('locked');
        return;
      }
      if (identity.active) {
        setState('ready');
        return;
      }
      setState('onboarding');
    } catch {
      setState('ready');
    }
  }, []);

  // Re-check identity whenever pathname changes (e.g., after onboarding completes)
  useEffect(() => {
    checkIdentity();
  }, [pathname, checkIdentity]);

  // Allow access to public paths always
  if (PUBLIC_PATHS.includes(pathname)) {
    return <>{children}</>;
  }

  if (state === 'loading') {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-center">
          <div className="w-10 h-10 border-2 border-blue-500 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
          <p className="text-gray-400">Connecting to node...</p>
        </div>
      </div>
    );
  }

  if (state === 'onboarding') {
    router.push('/onboarding');
    return null;
  }

  if (state === 'locked') {
    return (
      <PassphrasePrompt
        onUnlock={async (passphrase: string) => {
          await unlockIdentity({ passphrase });
          setState('ready');
        }}
        onCancel={() => {}}
      />
    );
  }

  return <>{children}</>;
}
