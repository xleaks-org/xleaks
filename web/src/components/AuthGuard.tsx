'use client';

import { useEffect, useState } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { getActiveIdentity, unlockIdentity } from '@/lib/api';
import PassphrasePrompt from './PassphrasePrompt';

const PUBLIC_PATHS = ['/onboarding'];

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [state, setState] = useState<'loading' | 'ready' | 'onboarding' | 'locked'>('loading');
  const [error, setError] = useState('');

  useEffect(() => {
    checkIdentity();
  }, []);

  async function checkIdentity() {
    try {
      const identity = await getActiveIdentity();
      if (!identity) {
        setState('onboarding');
        return;
      }
      if (identity.needsOnboarding) {
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
      // API not reachable — show the app anyway, it'll handle errors per-component
      setState('ready');
    }
  }

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
          setError('');
          try {
            await unlockIdentity({ passphrase });
            setState('ready');
          } catch (err) {
            throw err;
          }
        }}
        onCancel={() => {
          // Can't cancel — must unlock
        }}
      />
    );
  }

  return <>{children}</>;
}
