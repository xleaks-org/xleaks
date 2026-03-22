'use client';

import { useState, useCallback } from 'react';

export default function PassphrasePrompt({
  onUnlock,
  onCancel,
}: {
  onUnlock: (passphrase: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [passphrase, setPassphrase] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!passphrase.trim()) {
        setError('Please enter your passphrase');
        return;
      }
      setLoading(true);
      setError('');
      try {
        await onUnlock(passphrase);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Wrong passphrase. Try again.'
        );
      } finally {
        setLoading(false);
      }
    },
    [passphrase, onUnlock]
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-sm"
        onClick={onCancel}
      />

      {/* Modal */}
      <div className="relative bg-gray-900 border border-gray-800 rounded-2xl p-6 w-full max-w-md mx-4 shadow-2xl">
        {/* Close button */}
        <button
          onClick={onCancel}
          className="absolute top-4 right-4 text-gray-500 hover:text-white transition-colors"
          aria-label="Close"
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
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>

        {/* Lock icon */}
        <div className="w-14 h-14 mx-auto mb-4 rounded-full bg-blue-500/20 flex items-center justify-center">
          <svg
            className="w-7 h-7 text-blue-500"
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
        </div>

        <h2 className="text-xl font-bold text-white text-center mb-2">
          Unlock Your Identity
        </h2>
        <p className="text-sm text-gray-400 text-center mb-6">
          Enter your passphrase to access your identity
        </p>

        <form onSubmit={handleSubmit}>
          {error && (
            <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4 text-sm text-red-400">
              {error}
            </div>
          )}

          <input
            type="password"
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
            placeholder="Enter your passphrase"
            autoFocus
            className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors mb-4"
          />

          <button
            type="submit"
            disabled={loading || !passphrase.trim()}
            className="w-full bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-2.5 rounded-full transition-colors"
          >
            {loading ? 'Unlocking...' : 'Unlock'}
          </button>
        </form>
      </div>
    </div>
  );
}
