'use client';

import { useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { createIdentity, importIdentity, updateProfile } from '@/lib/api';

type Step = 'welcome' | 'create' | 'import' | 'seed' | 'seed_confirm' | 'passphrase' | 'profile' | 'done';

function pickRandomPositions(wordCount: number, count: number): number[] {
  const positions: number[] = [];
  while (positions.length < count) {
    const pos = Math.floor(Math.random() * wordCount);
    if (!positions.includes(pos)) {
      positions.push(pos);
    }
  }
  return positions.sort((a, b) => a - b);
}

export default function OnboardingPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>('welcome');
  const [seedPhrase, setSeedPhrase] = useState('');
  const [importSeed, setImportSeed] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [confirmPassphrase, setConfirmPassphrase] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [seedConfirmed, setSeedConfirmed] = useState(false);
  const [confirmPositions, setConfirmPositions] = useState<number[]>([]);
  const [confirmInputs, setConfirmInputs] = useState<Record<number, string>>({});

  const handleCreate = useCallback(async () => {
    if (!passphrase) {
      setError('Passphrase is required');
      return;
    }
    if (passphrase !== confirmPassphrase) {
      setError('Passphrases do not match');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const result = await createIdentity({ passphrase });
      setSeedPhrase(result.seedPhrase || result.mnemonic);
      setStep('seed');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create identity');
    } finally {
      setLoading(false);
    }
  }, [passphrase, confirmPassphrase]);

  const handleImport = useCallback(async () => {
    if (!importSeed.trim()) {
      setError('Seed phrase is required');
      return;
    }
    if (!passphrase) {
      setError('Passphrase is required');
      return;
    }
    setLoading(true);
    setError('');
    try {
      await importIdentity({ seedPhrase: importSeed, passphrase });
      setStep('profile');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to import identity');
    } finally {
      setLoading(false);
    }
  }, [importSeed, passphrase]);

  const handleSetProfile = useCallback(async () => {
    if (!displayName.trim()) {
      setError('Display name is required');
      return;
    }
    setLoading(true);
    setError('');
    try {
      await updateProfile({ displayName });
      setStep('done');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to set profile');
    } finally {
      setLoading(false);
    }
  }, [displayName]);

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        {/* Welcome */}
        {step === 'welcome' && (
          <div className="text-center">
            <h1 className="text-4xl font-bold text-white mb-2">XLeaks</h1>
            <p className="text-gray-400 mb-8">
              Decentralized. Peer-to-peer. Censorship-resistant.
            </p>
            <p className="text-gray-300 mb-8 text-sm">
              Your identity lives on your device. No email, no phone number, no
              central authority. Just you and the network.
            </p>
            <div className="space-y-3">
              <button
                onClick={() => setStep('passphrase')}
                className="w-full bg-blue-500 hover:bg-blue-600 text-white font-bold py-3 rounded-full transition-colors"
              >
                Create New Identity
              </button>
              <button
                onClick={() => setStep('import')}
                className="w-full bg-transparent border border-gray-600 hover:border-gray-500 text-white font-bold py-3 rounded-full transition-colors"
              >
                Import Existing Identity
              </button>
            </div>
          </div>
        )}

        {/* Set Passphrase (for create flow) */}
        {step === 'passphrase' && (
          <div>
            <h2 className="text-2xl font-bold text-white mb-2">
              Set Passphrase
            </h2>
            <p className="text-gray-400 mb-6 text-sm">
              This passphrase encrypts your identity on this device. Choose
              something strong and memorable.
            </p>

            {error && (
              <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4 text-sm text-red-400">
                {error}
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="block text-sm text-gray-400 mb-1">
                  Passphrase
                </label>
                <input
                  type="password"
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors"
                  placeholder="Enter a strong passphrase"
                />
              </div>
              <div>
                <label className="block text-sm text-gray-400 mb-1">
                  Confirm Passphrase
                </label>
                <input
                  type="password"
                  value={confirmPassphrase}
                  onChange={(e) => setConfirmPassphrase(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors"
                  placeholder="Confirm your passphrase"
                />
              </div>
            </div>

            <div className="flex gap-3 mt-6">
              <button
                onClick={() => {
                  setStep('welcome');
                  setError('');
                }}
                className="flex-1 border border-gray-600 text-white py-2.5 rounded-full hover:border-gray-500 transition-colors"
              >
                Back
              </button>
              <button
                onClick={handleCreate}
                disabled={loading}
                className="flex-1 bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-2.5 rounded-full transition-colors"
              >
                {loading ? 'Creating...' : 'Create Identity'}
              </button>
            </div>
          </div>
        )}

        {/* Import */}
        {step === 'import' && (
          <div>
            <h2 className="text-2xl font-bold text-white mb-2">
              Import Identity
            </h2>
            <p className="text-gray-400 mb-6 text-sm">
              Enter your seed phrase and set a passphrase to restore your
              identity on this device.
            </p>

            {error && (
              <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4 text-sm text-red-400">
                {error}
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="block text-sm text-gray-400 mb-1">
                  Seed Phrase
                </label>
                <textarea
                  value={importSeed}
                  onChange={(e) => setImportSeed(e.target.value)}
                  rows={3}
                  className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors resize-none"
                  placeholder="Enter your 12 or 24 word seed phrase"
                />
              </div>
              <div>
                <label className="block text-sm text-gray-400 mb-1">
                  New Passphrase
                </label>
                <input
                  type="password"
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors"
                  placeholder="Set a passphrase for this device"
                />
              </div>
            </div>

            <div className="flex gap-3 mt-6">
              <button
                onClick={() => {
                  setStep('welcome');
                  setError('');
                }}
                className="flex-1 border border-gray-600 text-white py-2.5 rounded-full hover:border-gray-500 transition-colors"
              >
                Back
              </button>
              <button
                onClick={handleImport}
                disabled={loading}
                className="flex-1 bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-2.5 rounded-full transition-colors"
              >
                {loading ? 'Importing...' : 'Import'}
              </button>
            </div>
          </div>
        )}

        {/* Seed Phrase Display */}
        {step === 'seed' && (
          <div>
            <h2 className="text-2xl font-bold text-white mb-2">
              Your Seed Phrase
            </h2>
            <p className="text-gray-400 mb-6 text-sm">
              Write this down and store it somewhere safe. This is the only way
              to recover your identity if you lose access to this device.
            </p>

            <div className="bg-gray-800 border border-gray-700 rounded-lg p-4 mb-4">
              <div className="grid grid-cols-3 gap-2">
                {seedPhrase.split(' ').map((word, i) => (
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

            <div className="bg-yellow-500/10 border border-yellow-500/30 rounded-lg p-3 mb-6">
              <p className="text-xs text-yellow-400">
                Warning: Never share your seed phrase. Anyone with it can take
                control of your identity. XLeaks will never ask for it.
              </p>
            </div>

            <label className="flex items-center gap-2 mb-6 cursor-pointer">
              <input
                type="checkbox"
                checked={seedConfirmed}
                onChange={(e) => setSeedConfirmed(e.target.checked)}
                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-500"
              />
              <span className="text-sm text-gray-300">
                I have written down my seed phrase and stored it safely
              </span>
            </label>

            <button
              onClick={() => {
                const words = seedPhrase.split(' ');
                const positions = pickRandomPositions(words.length, 3);
                setConfirmPositions(positions);
                setConfirmInputs({});
                setError('');
                setStep('seed_confirm');
              }}
              disabled={!seedConfirmed}
              className="w-full bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-3 rounded-full transition-colors"
            >
              Continue
            </button>
          </div>
        )}

        {/* Seed Phrase Confirmation */}
        {step === 'seed_confirm' && (
          <div>
            <h2 className="text-2xl font-bold text-white mb-2">
              Confirm Your Seed Phrase
            </h2>
            <p className="text-gray-400 mb-6 text-sm">
              Enter the missing words to verify you have saved your seed phrase
              correctly.
            </p>

            {error && (
              <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4 text-sm text-red-400">
                {error}
              </div>
            )}

            <div className="bg-gray-800 border border-gray-700 rounded-lg p-4 mb-6">
              <div className="grid grid-cols-3 gap-2">
                {seedPhrase.split(' ').map((word, i) => {
                  const isBlank = confirmPositions.includes(i);
                  return (
                    <div
                      key={i}
                      className="bg-gray-900 rounded px-2 py-1.5 text-sm text-center"
                    >
                      <span className="text-gray-500 mr-1">{i + 1}.</span>
                      {isBlank ? (
                        <input
                          type="text"
                          value={confirmInputs[i] ?? ''}
                          onChange={(e) =>
                            setConfirmInputs((prev) => ({
                              ...prev,
                              [i]: e.target.value.toLowerCase().trim(),
                            }))
                          }
                          className="bg-gray-800 border border-gray-600 rounded px-1 py-0.5 text-white text-sm w-16 outline-none focus:border-blue-500 text-center"
                          placeholder="?"
                          autoComplete="off"
                        />
                      ) : (
                        <span className="text-white">{word}</span>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>

            <div className="flex gap-3">
              <button
                onClick={() => {
                  setStep('seed');
                  setError('');
                }}
                className="flex-1 border border-gray-600 text-white py-2.5 rounded-full hover:border-gray-500 transition-colors"
              >
                Back
              </button>
              <button
                onClick={() => {
                  const words = seedPhrase.split(' ');
                  const allCorrect = confirmPositions.every(
                    (pos) => confirmInputs[pos] === words[pos]
                  );
                  if (!allCorrect) {
                    setError(
                      'Some words do not match. Please check and try again.'
                    );
                    return;
                  }
                  setError('');
                  setStep('profile');
                }}
                disabled={
                  confirmPositions.some(
                    (pos) => !confirmInputs[pos]?.trim()
                  )
                }
                className="flex-1 bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-2.5 rounded-full transition-colors"
              >
                Verify
              </button>
            </div>
          </div>
        )}

        {/* Set Display Name */}
        {step === 'profile' && (
          <div>
            <h2 className="text-2xl font-bold text-white mb-2">
              Set Your Name
            </h2>
            <p className="text-gray-400 mb-6 text-sm">
              Choose a display name for your profile. You can change this later.
            </p>

            {error && (
              <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4 text-sm text-red-400">
                {error}
              </div>
            )}

            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 outline-none focus:border-blue-500 transition-colors mb-6"
              placeholder="Your display name"
              maxLength={50}
            />

            <button
              onClick={handleSetProfile}
              disabled={loading || !displayName.trim()}
              className="w-full bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold py-3 rounded-full transition-colors"
            >
              {loading ? 'Saving...' : 'Finish Setup'}
            </button>
          </div>
        )}

        {/* Done */}
        {step === 'done' && (
          <div className="text-center">
            <div className="w-16 h-16 mx-auto mb-4 rounded-full bg-green-500/20 flex items-center justify-center">
              <svg className="w-8 h-8 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            </div>
            <h2 className="text-2xl font-bold text-white mb-2">
              You are all set!
            </h2>
            <p className="text-gray-400 mb-8 text-sm">
              Your decentralized identity is ready. Welcome to XLeaks.
            </p>
            <button
              onClick={() => router.push('/')}
              className="w-full bg-blue-500 hover:bg-blue-600 text-white font-bold py-3 rounded-full transition-colors"
            >
              Go to Home
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
