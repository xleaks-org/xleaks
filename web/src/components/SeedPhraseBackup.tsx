'use client';

import { useState, useCallback } from 'react';

export default function SeedPhraseBackup({
  seedPhrase,
  onConfirm,
}: {
  seedPhrase: string;
  onConfirm: () => void;
}) {
  const [confirmed, setConfirmed] = useState(false);
  const [copied, setCopied] = useState(false);

  const words = seedPhrase.split(' ');

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(seedPhrase);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard not available
    }
  }, [seedPhrase]);

  return (
    <div>
      <h2 className="text-2xl font-bold text-white mb-2">Your Seed Phrase</h2>

      {/* Warning */}
      <div className="bg-yellow-500/10 border border-yellow-500/30 rounded-lg p-3 mb-4">
        <p className="text-sm text-yellow-400 font-medium">
          Write these words down. If you lose them, you lose access to your
          identity forever.
        </p>
      </div>

      {/* Seed phrase grid */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-4 mb-4">
        <div className="grid grid-cols-4 gap-2">
          {words.map((word, i) => (
            <div
              key={i}
              className="bg-gray-900 rounded px-2 py-1.5 text-sm text-center"
            >
              <span className="text-gray-500 mr-1">{i + 1}.</span>
              <span className="text-white">{word}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Copy button */}
      <button
        onClick={handleCopy}
        className="text-sm text-blue-500 hover:text-blue-400 transition-colors mb-4 flex items-center gap-1.5"
      >
        <svg
          className="w-4 h-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9.75a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184"
          />
        </svg>
        {copied ? 'Copied!' : 'Copy to clipboard'}
      </button>

      {/* Warning notice */}
      <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-6">
        <p className="text-xs text-red-400">
          Never share your seed phrase. Anyone with it can take control of your
          identity. XLeaks will never ask for it.
        </p>
      </div>

      {/* Confirm checkbox */}
      <label className="flex items-center gap-2 mb-6 cursor-pointer">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={(e) => setConfirmed(e.target.checked)}
          className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-500"
        />
        <span className="text-sm text-gray-300">
          I have saved my seed phrase and stored it safely
        </span>
      </label>

      {/* Continue */}
      <button
        onClick={onConfirm}
        disabled={!confirmed}
        className="w-full bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-3 rounded-full transition-colors"
      >
        Continue
      </button>
    </div>
  );
}
